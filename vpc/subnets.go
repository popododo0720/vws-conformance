package vpc

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Subnets() bind.Group {
	return bind.Group{Service: "vpc", Name: "subnets", Run: runSubs, Cases: []cases.Case{
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
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "VPC-SUB-103":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeSubnets(ctx, &awsec2.DescribeSubnetsInput{
				SubnetIds: []string{"subnet-0123456789abcdef0"},
			})
			return wrapErr("runSubs", err)
		})
		return o, wrapErr("runSubs", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
