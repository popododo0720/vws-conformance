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
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func SecurityGroups() bind.Group {
	return bind.Group{Service: "vpc", Name: "securitygroups", Run: runSG, Cases: []cases.Case{
		{
			ID: "VPC-SG-001", Service: "vpc", Group: "securitygroups", Op: "CreateSecurityGroup", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateSecurityGroup"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-SG-002", Service: "vpc", Group: "securitygroups", Op: "AuthorizeSecurityGroupIngress",
			Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("AuthorizeSecurityGroupIngress"),
			Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-SG-003", Service: "vpc", Group: "securitygroups", Op: "DescribeSecurityGroups", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeSecurityGroups"), Expect: cases.Expect{HTTP: 200},
			Impl: "yes",
		},
		{
			ID: "VPC-SG-004", Service: "vpc", Group: "securitygroups", Op: "DeleteSecurityGroup", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteSecurityGroup"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "VPC-SG-101", Service: "vpc", Group: "securitygroups", Op: "CreateSecurityGroup", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidGroup.Duplicate"},
			Impl: "yes",
		},
		{
			ID: "VPC-SG-102", Service: "vpc", Group: "securitygroups", Op: "DeleteSecurityGroup", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"},
			Impl: "yes",
		},
		{
			ID: "VPC-SG-103", Service: "vpc", Group: "securitygroups", Op: "AuthorizeSecurityGroupIngress",
			Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidPermission.Duplicate",
			}, Impl: "yes",
		},
		{
			ID: "VPC-SG-104", Service: "vpc", Group: "securitygroups", Op: "DescribeSecurityGroups", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"},
			Impl: "yes",
		},
	}}
}

func runSG(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := api(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	if c.ID == "VPC-SG-102" || c.ID == "VPC-SG-104" {
		cidr := "10.24.0.0/16"
		if c.ID == "VPC-SG-104" {
			cidr = "10.25.0.0/16"
		}
		id, err := goneGroup(ctx, rt, api, cidr, rt.Name(strings.ToLower(c.ID)))
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				var err error
				if c.ID == "VPC-SG-104" {
					_, err = api.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{GroupIds: []string{id}})
				} else {
					_, err = api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
				}
				return wrapErr("runSG", err)
			})
			return callOut, wrapErr("runSG", callErr)
		}
	}
	vpcID, err := makeVpc(ctx, rt, api, "10.13.0.0/16", "sgvpc")
	if err != nil {
		return cases.Outcome{}, err
	}
	name := rt.Name(strings.ToLower(c.ID))
	switch c.ID {
	case "VPC-SG-001":
		var id string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
				GroupName:   aws.String(name),
				Description: aws.String("conformance"), VpcId: aws.String(vpcID),
			})
			if err == nil && out != nil {
				id = aws.ToString(out.GroupId)
			}
			return wrapErr("runSG", err)
		})
		if id != "" {
			rt.Cleanup(func(ctx context.Context) error {
				return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
					_, err := api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
					return ignoreGone(err)
				})
			})
		}
		return o, wrapErr("runSG", err)
	case "VPC-SG-002", "VPC-SG-103":
		id, err := makeSG(ctx, rt, api, vpcID, name)
		if err != nil {
			return cases.Outcome{}, err
		}
		ip := types.IpPermission{
			IpProtocol: aws.String("tcp"), FromPort: aws.Int32(22), ToPort: aws.Int32(22),
			IpRanges: []types.IpRange{{CidrIp: aws.String("10.0.0.0/8")}},
		}
		if c.ID == "VPC-SG-103" {
			if _, err := api.AuthorizeSecurityGroupIngress(ctx,
				&awsec2.AuthorizeSecurityGroupIngressInput{
					GroupId:       aws.String(id),
					IpPermissions: []types.IpPermission{ip},
				}); err != nil {
				return cases.Outcome{}, fmt.Errorf("first authorize: %w", err)
			}
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.AuthorizeSecurityGroupIngress(ctx, &awsec2.AuthorizeSecurityGroupIngressInput{
					GroupId:       aws.String(id),
					IpPermissions: []types.IpPermission{ip},
				})
				return wrapErr("runSG", err)
			})
			return callOut, wrapErr("runSG", callErr)
		}
	case "VPC-SG-003":
		id, err := makeSG(ctx, rt, api, vpcID, name)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeSecurityGroups(ctx, &awsec2.DescribeSecurityGroupsInput{GroupIds: []string{id}})
				return wrapErr("runSG", err)
			})
			return callOut, wrapErr("runSG", callErr)
		}
	case "VPC-SG-004":
		id, err := makeSG(ctx, rt, api, vpcID, name)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
				return wrapErr("runSG", err)
			})
			return callOut, wrapErr("runSG", callErr)
		}
	case "VPC-SG-101":
		if _, err := makeSG(ctx, rt, api, vpcID, name); err != nil {
			return cases.Outcome{}, err
		}
		{
			var created string
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				out, err := api.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
					GroupName:   aws.String(name),
					Description: aws.String("conformance"), VpcId: aws.String(vpcID),
				})
				if err == nil && out != nil {
					created = aws.ToString(out.GroupId)
				}
				return wrapErr("runSG", err)
			})
			if created != "" {
				if err := rt.Track("security-group", created, name); err != nil {
					return callOut, wrapErr("runSG", err)
				}
				rt.Cleanup(func(ctx context.Context) error {
					return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
						_, err := api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(created)})
						return ignoreGone(err)
					})
				})
			}
			return callOut, wrapErr("runSG", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func goneGroup(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, cidr, name string) (string, error) {
	vpcID, err := makeVpc(ctx, rt, api, cidr, "sgmiss")
	if err != nil {
		return "", err
	}
	id, err := makeSG(ctx, rt, api, vpcID, name)
	if err != nil {
		return "", err
	}
	if _, err := api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(id)}); err != nil {
		return "", fmt.Errorf("delete security group %s: %w", id, err)
	}
	return id, nil
}

func makeSG(ctx context.Context, rt *harness.Runtime, api *awsec2.Client, vpcID, name string) (string, error) {
	out, err := api.CreateSecurityGroup(ctx, &awsec2.CreateSecurityGroupInput{
		GroupName:   aws.String(name),
		Description: aws.String("conformance"), VpcId: aws.String(vpcID),
	})
	if err != nil {
		return "", fmt.Errorf("create security group: %w", err)
	}
	id := aws.ToString(out.GroupId)
	rt.Cleanup(func(ctx context.Context) error {
		return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
			_, err := api.DeleteSecurityGroup(ctx, &awsec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
			return ignoreGone(err)
		})
	})
	return id, nil
}
