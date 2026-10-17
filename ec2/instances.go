package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Instances() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "instances", Run: runInstances,
		Cases: []cases.Case{
			{
				ID: "EC2-DI-101", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-DI-102", Service: "ec2", Group: "instances", Op: "DescribeInstances", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.Malformed",
				}, Impl: "yes",
			},
			{
				ID: "EC2-DIA-101", Service: "ec2", Group: "instances", Op: "DescribeInstanceAttribute",
				Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidInstanceID.NotFound",
				}, Impl: "yes",
			},
		},
	}
}

func runInstances(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-DI-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{
				InstanceIds: []string{"i-0123456789abcdef0"},
			})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	case "EC2-DI-102":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstances(ctx, &awsec2.DescribeInstancesInput{InstanceIds: []string{"i-xyz"}})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	case "EC2-DIA-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceAttribute(ctx, &awsec2.DescribeInstanceAttributeInput{
				InstanceId: aws.String("i-0123456789abcdef0"),
				Attribute:  types.InstanceAttributeNameUserData,
			})
			return wrapErr("runInstances", err)
		})
		return o, wrapErr("runInstances", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
