package vpc

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Vpcs() bind.Group {
	return bind.Group{Service: "vpc", Name: "vpcs", Run: runVpcs, Cases: []cases.Case{
		{
			ID: "VPC-VPC-001", Service: "vpc", Group: "vpcs", Op: "CreateVpc", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2("CreateVpc"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"id": expect.VpcID}},
			Impl: "yes",
		},
		{
			ID: "VPC-VPC-002", Service: "vpc", Group: "vpcs", Op: "DescribeVpcs", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2("DescribeVpcs"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-VPC-003", Service: "vpc", Group: "vpcs", Op: "DeleteVpc", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2("DeleteVpc"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-VPC-101", Service: "vpc", Group: "vpcs", Op: "CreateVpc", Kind: cases.KindError, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVpc.Range"}, Impl: "yes",
		},
		{
			ID: "VPC-VPC-102", Service: "vpc", Group: "vpcs", Op: "DeleteVpc", Kind: cases.KindError, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVpcID.NotFound"}, Impl: "yes",
		},
		{
			ID: "VPC-VPC-103", Service: "vpc", Group: "vpcs", Op: "DeleteVpc", Kind: cases.KindError, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "DependencyViolation"}, Impl: "yes",
		},
		{
			ID: "VPC-VPC-104", Service: "vpc", Group: "vpcs", Op: "DescribeVpcs", Kind: cases.KindError, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVpcID.NotFound"}, Impl: "yes",
		},
	}}
}

func runVpcs(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := api(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-VPC-001":
		return createVpc(ctx, rt, api, "10.8.0.0/16", "vpc1")
	case "VPC-VPC-002":
		id, err := makeVpc(ctx, rt, api, "10.9.0.0/16", "vpc2")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{VpcIds: []string{id}})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	case "VPC-VPC-003":
		id, err := makeVpc(ctx, rt, api, "10.10.0.0/16", "vpc3")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteVpc(ctx, &awsec2.DeleteVpcInput{VpcId: aws.String(id)})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	case "VPC-VPC-101":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateVpc(ctx, &awsec2.CreateVpcInput{CidrBlock: aws.String("10.0.0.0/33")})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	case "VPC-VPC-102":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteVpc(ctx, &awsec2.DeleteVpcInput{VpcId: aws.String("vpc-0123456789abcdef0")})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	case "VPC-VPC-104":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeVpcs(ctx, &awsec2.DescribeVpcsInput{VpcIds: []string{"vpc-0123456789abcdef0"}})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	case "VPC-VPC-103":
		id, err := makeVpc(ctx, rt, api, "10.11.0.0/16", "vpc4")
		if err != nil {
			return cases.Outcome{}, err
		}
		if _, err := createSubnet(ctx, rt, api, id, "10.11.1.0/24", "vpc4sub"); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteVpc(ctx, &awsec2.DeleteVpcInput{VpcId: aws.String(id)})
				return wrapErr("runVpcs", err)
			})
			return callOut, wrapErr("runVpcs", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func api(rt *harness.Runtime) (*awsec2.Client, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, fmt.Errorf("ec2 client is unset")
	}
	return rt.Clients.EC2, nil
}

func createVpc(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, cidr, token string) (cases.Outcome,
	error,
) {
	var id string
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		out, err := api.CreateVpc(ctx, &awsec2.CreateVpcInput{
			CidrBlock:         aws.String(cidr),
			TagSpecifications: rt.TagSpec(types.ResourceTypeVpc),
		})
		if err == nil && out.Vpc != nil {
			id = aws.ToString(out.Vpc.VpcId)
		}
		return wrapErr("createVpc", err)
	})
	o.Fields["id"] = id
	if id != "" {
		if err := rt.Track("vpc", id, rt.Name(token)); err != nil {
			return o, wrapErr("createVpc", err)
		}
		rt.Cleanup(func(ctx context.Context) error {
			_, err := api.DeleteVpc(ctx, &awsec2.DeleteVpcInput{VpcId: aws.String(id)})
			return ignoreGone(err)
		})
	}
	return o, wrapErr("createVpc", err)
}

func makeVpc(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, cidr, token string) (string, error) {
	out, err := api.CreateVpc(ctx, &awsec2.CreateVpcInput{
		CidrBlock:         aws.String(cidr),
		TagSpecifications: rt.TagSpec(types.ResourceTypeVpc),
	})
	if err != nil {
		return "", fmt.Errorf("create vpc: %w", err)
	}
	id := aws.ToString(out.Vpc.VpcId)
	if err := rt.Track("vpc", id, rt.Name(token)); err != nil {
		return "", wrapErr("makeVpc", err)
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := api.DeleteVpc(ctx, &awsec2.DeleteVpcInput{VpcId: aws.String(id)})
		return ignoreGone(err)
	})
	return id, nil
}
