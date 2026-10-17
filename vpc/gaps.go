package vpc

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{Service: "vpc", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		vpcGap("VPC-GAP-004", "DescribeAvailabilityZones"),
		vpcGap("VPC-GAP-005", "DescribeRegions"),
		vpcGap("VPC-GAP-007", "DescribeRouteTables"),
	}}
}

func vpcGap(id, op string) cases.Case {
	return cases.Case{
		ID: id, Service: "vpc", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK,
		DocURL: cases.DocEC2(op), Expect: cases.Expect{HTTP: 200}, Impl: "gap",
	}
}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-GAP-004":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeAvailabilityZones(ctx, &awsec2.DescribeAvailabilityZonesInput{})
			return wrapErr("runGaps", err)
		})
		return o, wrapErr("runGaps", err)
	case "VPC-GAP-005":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRegions(ctx, &awsec2.DescribeRegionsInput{})
			return wrapErr("runGaps", err)
		})
		return o, wrapErr("runGaps", err)
	case "VPC-GAP-007":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{})
			return wrapErr("runGaps", err)
		})
		return o, wrapErr("runGaps", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
