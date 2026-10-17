package vpc

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Vpcs() bind.Group {
	return bind.Group{Service: "vpc", Name: "vpcs", Run: runVpcs, Cases: []cases.Case{
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
