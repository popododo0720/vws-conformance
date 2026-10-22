package ec2

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Addresses() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "addresses", Run: runAddr,
		Cases: []cases.Case{
			{
				ID: "EC2-EIP-103", Service: "ec2", Group: "addresses", Op: "DescribeAddresses", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidAllocationID.NotFound",
				}, Impl: "yes",
			},
		},
	}
}

func runAddr(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-EIP-103":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeAddresses(ctx, &awsec2.DescribeAddressesInput{
				AllocationIds: []string{"eipalloc-0123456789abcdef0"},
			})
			return wrapErr("runAddr", err)
		})
		return o, wrapErr("runAddr", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
