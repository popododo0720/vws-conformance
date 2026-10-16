package vpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Subnets() bind.Group {
	return bind.Group{Service: "vpc", Name: "subnets", Run: runSubs, Cases: []cases.Case{
		{
			ID: "VPC-SUB-001", Service: "vpc", Group: "subnets", Op: "CreateSubnet", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateSubnet"), Expect: cases.Expect{
				HTTP:     200,
				Patterns: map[string]string{"id": expect.SubnetID, "zone": "vws-1a"},
			}, Impl: "yes",
		},
		{
			ID: "VPC-SUB-002", Service: "vpc", Group: "subnets", Op: "DescribeSubnets", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeSubnets"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-SUB-003", Service: "vpc", Group: "subnets", Op: "DeleteSubnet", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteSubnet"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-SUB-101", Service: "vpc", Group: "subnets", Op: "CreateSubnet", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidSubnet.Range"},
			Impl: "yes",
		},
		{
			ID: "VPC-SUB-102", Service: "vpc", Group: "subnets", Op: "DeleteSubnet", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidSubnetID.NotFound",
			}, Impl: "yes",
		},
		{
			ID: "VPC-SUB-103", Service: "vpc", Group: "subnets", Op: "DescribeSubnets", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidSubnetID.NotFound",
			}, Impl: "yes",
		},
	}}
}

func runSubs(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := api(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	if c.ID == "VPC-SUB-102" || c.ID == "VPC-SUB-103" {
		cidr, sub := "10.22.0.0/16", "10.22.1.0/24"
		if c.ID == "VPC-SUB-103" {
			cidr, sub = "10.23.0.0/16", "10.23.1.0/24"
		}
		id, err := goneSubnet(ctx, rt, api, cidr, sub, "submiss")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				var err error
				if c.ID == "VPC-SUB-103" {
					_, err = api.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{SubnetIds: []string{id}})
				} else {
					_, err = api.DeleteSubnet(ctx, &awsec2.DeleteSubnetInput{SubnetId: aws.String(id)})
				}
				return wrapErr("runSubs", err)
			})
			return callOut, wrapErr("runSubs", callErr)
		}
	}
	vpcID, err := makeVpc(ctx, rt, api, "10.12.0.0/16", "subvpc")
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-SUB-001":
		var id, zone string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.CreateSubnet(ctx, subnetInput(rt, vpcID, "10.12.1.0/24"))
			if err == nil && out.Subnet != nil {
				id = aws.ToString(out.Subnet.SubnetId)
				zone = aws.ToString(out.Subnet.AvailabilityZone)
			}
			return wrapErr("runSubs", err)
		})
		if id != "" {
			if err := ownSubnet(rt, api, id, "sub1"); err != nil {
				return o, wrapErr("runSubs", err)
			}
		}
		o.Fields["id"] = id
		o.Fields["zone"] = zone
		return o, wrapErr("runSubs", err)
	case "VPC-SUB-002":
		id, err := createSubnet(ctx, rt, api, vpcID, "10.12.2.0/24", "sub2")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{SubnetIds: []string{id}})
				return wrapErr("runSubs", err)
			})
			return callOut, wrapErr("runSubs", callErr)
		}
	case "VPC-SUB-003":
		id, err := createSubnet(ctx, rt, api, vpcID, "10.12.3.0/24", "sub3")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteSubnet(ctx, &awsec2.DeleteSubnetInput{SubnetId: aws.String(id)})
				return wrapErr("runSubs", err)
			})
			return callOut, wrapErr("runSubs", callErr)
		}
	case "VPC-SUB-101":
		var id string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.CreateSubnet(ctx, subnetInput(rt, vpcID, "10.99.0.0/24"))
			if err == nil && out != nil && out.Subnet != nil {
				id = aws.ToString(out.Subnet.SubnetId)
			}
			return wrapErr("runSubs", err)
		})
		if id != "" {
			if err := ownSubnet(rt, api, id, "sub101"); err != nil {
				return o, wrapErr("runSubs", err)
			}
		}
		return o, wrapErr("runSubs", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func subnetInput(rt *harness.Runtime, vpcID, cidr string) *awsec2.CreateSubnetInput {
	return &awsec2.CreateSubnetInput{
		VpcId:             aws.String(vpcID),
		CidrBlock:         aws.String(cidr),
		AvailabilityZone:  aws.String("vws-1a"),
		TagSpecifications: rt.TagSpec(types.ResourceTypeSubnet),
	}
}

func goneSubnet(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, cidr, sub, token string) (string,
	error,
) {
	vpcID, err := makeVpc(ctx, rt, api, cidr, token+"vpc")
	if err != nil {
		return "", err
	}
	id, err := createSubnet(ctx, rt, api, vpcID, sub, token)
	if err != nil {
		return "", err
	}
	if _, err := api.DeleteSubnet(ctx, &awsec2.DeleteSubnetInput{SubnetId: aws.String(id)}); err != nil {
		return "", fmt.Errorf("delete subnet %s: %w", id, err)
	}
	return id, nil
}

func createSubnet(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, vpcID, cidr, token string) (string,
	error,
) {
	out, err := api.CreateSubnet(ctx, subnetInput(rt, vpcID, cidr))
	if err != nil {
		return "", fmt.Errorf("create subnet: %w", err)
	}
	id := aws.ToString(out.Subnet.SubnetId)
	if err := ownSubnet(rt, api, id, token); err != nil {
		return "", err
	}
	return id, nil
}

func ownSubnet(rt *harness.Runtime, api *awsec2.Client, id, token string) error {
	return wrapErr("ownSubnet", rt.Own("subnet", id, rt.Name(token), func(ctx context.Context) error {
		_, err := api.DeleteSubnet(ctx, &awsec2.DeleteSubnetInput{SubnetId: aws.String(id)})
		if err == nil || strings.Contains(err.Error(), "NotFound") {
			return nil
		}
		return wrapErr("ownSubnet", err)
	}))
}
