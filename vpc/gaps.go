package vpc

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

func Gaps() bind.Group {
	return bind.Group{Service: "vpc", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		vpcGap("VPC-GAP-001", "RevokeSecurityGroupIngress", cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"}),
		vpcGap("VPC-GAP-002", "AuthorizeSecurityGroupEgress", cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"}),
		vpcGap("VPC-GAP-003", "RevokeSecurityGroupEgress", cases.Expect{HTTP: 400, Code: "InvalidGroup.NotFound"}),
		vpcGap("VPC-GAP-004", "DescribeAvailabilityZones", cases.Expect{HTTP: 200}),
		vpcGap("VPC-GAP-005", "DescribeRegions", cases.Expect{HTTP: 200}),
		vpcGap("VPC-GAP-006", "CreateInternetGateway", cases.Expect{HTTP: 200}),
		vpcGap("VPC-GAP-007", "DescribeRouteTables", cases.Expect{HTTP: 200}),
	}}
}

func vpcGap(id, op string, exp cases.Expect) cases.Case {
	return cases.Case{ID: id, Service: "vpc", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocEC2(op), Expect: exp, Impl: "gap"}
}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := api(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	gid := aws.String("sg-0123456789abcdef0")
	switch c.ID {
	case "VPC-GAP-001":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.RevokeSecurityGroupIngress(ctx, &awsec2.RevokeSecurityGroupIngressInput{GroupId: gid})
			return err
		})
	case "VPC-GAP-002":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.AuthorizeSecurityGroupEgress(ctx, &awsec2.AuthorizeSecurityGroupEgressInput{GroupId: gid, IpPermissions: []types.IpPermission{{IpProtocol: aws.String("-1"), IpRanges: []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}}}}})
			return err
		})
	case "VPC-GAP-003":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.RevokeSecurityGroupEgress(ctx, &awsec2.RevokeSecurityGroupEgressInput{GroupId: gid})
			return err
		})
	case "VPC-GAP-004":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeAvailabilityZones(ctx, &awsec2.DescribeAvailabilityZonesInput{})
			return err
		})
	case "VPC-GAP-005":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRegions(ctx, &awsec2.DescribeRegionsInput{})
			return err
		})
	case "VPC-GAP-006":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateInternetGateway(ctx, &awsec2.CreateInternetGatewayInput{})
			return err
		})
	case "VPC-GAP-007":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeRouteTables(ctx, &awsec2.DescribeRouteTablesInput{})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
