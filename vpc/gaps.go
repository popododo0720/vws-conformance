package vpc

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{Service: "vpc", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		vpcGap("VPC-GAP-004", "DescribeAvailabilityZones"),
		vpcGap("VPC-GAP-005", "DescribeRegions"),
		{
			ID: "VPC-GAP-006", Service: "vpc", Group: "gaps", Op: "CreateInternetGateway", Kind: cases.KindGap,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateInternetGateway"), Impl: "yes",
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"internetGatewayId": "^igw-[0-9a-f]+$"}},
		},
		vpcGap("VPC-GAP-007", "DescribeRouteTables"),
	}}
}

func vpcGap(id, op string) cases.Case {
	return cases.Case{
		ID: id, Service: "vpc", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK,
		DocURL: cases.DocEC2(op), Expect: shapeExpect, Impl: "gap",
	}
}

// shapeExpect is the AWS response-shape expectation for the read-only gap cases.
var shapeExpect = cases.Expect{HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern}}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-GAP-006":
		var created *awsec2.CreateInternetGatewayOutput
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			var callErr error
			created, callErr = rt.EC2CreateInternetGateway(ctx, &awsec2.CreateInternetGatewayInput{})
			return wrapErr("runGaps", callErr)
		})
		if created != nil && created.InternetGateway != nil {
			o.Fields["internetGatewayId"] = aws.ToString(created.InternetGateway.InternetGatewayId)
		}
		return o, wrapErr("runGaps", err)
	case "VPC-GAP-004":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeAvailabilityZones(ctx, &awsec2.DescribeAvailabilityZonesInput{})
			return wrapErr("runGaps", err)
		})
		o.Fields["shape"] = expect.ShapeField(
			o, "DescribeAvailabilityZones", "availabilityZoneInfo",
			"zoneName", "zoneId", "regionName",
		)
		return o, wrapErr("runGaps", err)
	case "VPC-GAP-005":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRegions(ctx, &awsec2.DescribeRegionsInput{})
			return wrapErr("runGaps", err)
		})
		o.Fields["shape"] = expect.ShapeField(
			o, "DescribeRegions", "regionInfo",
			"regionName", "regionEndpoint",
		)
		return o, wrapErr("runGaps", err)
	case "VPC-GAP-007":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{})
			return wrapErr("runGaps", err)
		})
		o.Fields["shape"] = expect.ShapeField(
			o, "DescribeRouteTables", "routeTableSet",
			"routeTableId", "vpcId",
		)
		return o, wrapErr("runGaps", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
