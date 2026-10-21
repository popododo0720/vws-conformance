package ec2

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "gaps", Run: runEC2Gaps,
		Cases: []cases.Case{
			ec2Gap("EC2-GAP-003", "DescribeInstanceStatus"),
			ec2Gap("EC2-GAP-004", "DescribeInstanceTypes"),
			ec2Gap("EC2-GAP-008", "DescribeTags"),
		},
	}
}

func ec2Gap(id, op string) cases.Case {
	return cases.Case{
		ID: id, Service: "ec2", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK,
		DocURL: cases.DocEC2(op), Expect: cases.Expect{HTTP: 200}, Impl: "gap",
	}
}

func runEC2Gaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-GAP-003":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceStatus(ctx, &awsec2.DescribeInstanceStatusInput{})
			return wrapErr("runEC2Gaps", err)
		})
		return o, wrapErr("runEC2Gaps", err)
	case "EC2-GAP-004":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeInstanceTypes(ctx, &awsec2.DescribeInstanceTypesInput{})
			return wrapErr("runEC2Gaps", err)
		})
		return o, wrapErr("runEC2Gaps", err)
	case "EC2-GAP-008":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeTags(ctx, &awsec2.DescribeTagsInput{})
			return wrapErr("runEC2Gaps", err)
		})
		return o, wrapErr("runEC2Gaps", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
