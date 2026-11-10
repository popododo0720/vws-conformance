package vpc

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Vpcs() bind.Group {
	return bind.Group{Service: "vpc", Name: "vpcs", Run: runVpcs, Cases: []cases.Case{
		{
			ID: "VPC-VPC-001", Service: "vpc", Group: "vpcs", Op: "CreateVpc", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateVpc"), Impl: "yes",
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{
				"vpcId": "^vpc-[0-9a-f]+$", "cidrBlock": "^10\\.44\\.0\\.0/16$",
			}},
		},
		{
			ID: "VPC-VPC-002", Service: "vpc", Group: "vpcs", Op: "DescribeVpcs", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVpcs"), Impl: "yes",
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{
				"matches": "^true$", "cidrBlock": "^10\\.44\\.0\\.0/16$",
			}},
		},
		{
			ID: "VPC-VPC-003", Service: "vpc", Group: "vpcs", Op: "DeleteVpc", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteVpc"), Impl: "yes",
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"return": "^true$"}},
		},
		{
			ID: "VPC-VPC-101", Service: "vpc", Group: "vpcs", Op: "CreateVpc", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Impl: "yes",
			Expect: cases.Expect{HTTP: 400, Code: "InvalidVpcRange"},
		},
		{
			ID: "VPC-VPC-102", Service: "vpc", Group: "vpcs", Op: "DeleteVpc", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Impl: "yes",
			Expect: cases.Expect{HTTP: 400, Code: "InvalidVpcID.NotFound"},
		},
		{
			ID: "VPC-VPC-104", Service: "vpc", Group: "vpcs", Op: "DescribeVpcs", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
			Expect: cases.Expect{HTTP: 400, Code: "InvalidVpcID.NotFound"}, Impl: "yes",
		},
	}}
}

func runVpcs(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-VPC-001", "VPC-VPC-101":
		cidr := "10.44.0.0/16"
		if c.ID == "VPC-VPC-101" {
			cidr = "10.44.0.0/29"
		}
		var created *awsec2.CreateVpcOutput
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			var callErr error
			created, callErr = rt.EC2CreateVpc(ctx, &awsec2.CreateVpcInput{CidrBlock: aws.String(cidr)})
			return wrapErr("runVpcs", callErr)
		})
		if created != nil && created.Vpc != nil {
			o.Fields["vpcId"] = aws.ToString(created.Vpc.VpcId)
			o.Fields["cidrBlock"] = aws.ToString(created.Vpc.CidrBlock)
		}
		return o, wrapErr("runVpcs", err)
	case "VPC-VPC-002", "VPC-VPC-003", "VPC-VPC-102":
		created, err := rt.EC2CreateVpc(ctx, &awsec2.CreateVpcInput{CidrBlock: aws.String("10.44.0.0/16")})
		if err != nil {
			return cases.Outcome{}, wrapErr("create VPC fixture", err)
		}
		id := aws.ToString(created.Vpc.VpcId)
		if c.ID == "VPC-VPC-002" {
			var described *awsec2.DescribeVpcsOutput
			o, err := rt.Call(ctx, func(ctx context.Context) error {
				var callErr error
				described, callErr = api.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{VpcIds: []string{id}})
				return wrapErr("runVpcs", callErr)
			})
			if described != nil && len(described.Vpcs) == 1 {
				o.Fields["matches"] = fmt.Sprint(aws.ToString(described.Vpcs[0].VpcId) == id)
				o.Fields["cidrBlock"] = aws.ToString(described.Vpcs[0].CidrBlock)
			}
			return o, wrapErr("runVpcs", err)
		}
		if c.ID == "VPC-VPC-102" {
			if _, err := rt.EC2DeleteVpc(ctx, id); err != nil {
				return cases.Outcome{}, wrapErr("delete VPC fixture", err)
			}
		}
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			var callErr error
			_, callErr = rt.EC2DeleteVpc(ctx, id)
			return wrapErr("runVpcs", callErr)
		})
		if o.HTTP == 200 {
			var response struct {
				XMLName xml.Name `xml:"DeleteVpcResponse"`
				Return  string   `xml:"return"`
			}
			if parseErr := xml.Unmarshal(o.Body, &response); parseErr != nil {
				return o, fmt.Errorf("decode DeleteVpc response: %w", parseErr)
			}
			o.Fields["return"] = response.Return
		}
		return o, wrapErr("runVpcs", err)
	case "VPC-VPC-104":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{VpcIds: []string{"vpc-0123456789abcdef0"}})
			return wrapErr("runVpcs", err)
		})
		return o, wrapErr("runVpcs", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
