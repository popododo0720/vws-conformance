package ec2

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func KeyPairs() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "keypairs", Run: runKeys,
		Cases: []cases.Case{
			{
				ID: "EC2-KP-103", Service: "ec2", Group: "keypairs", Op: "DescribeKeyPairs", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidKeyPair.NotFound",
				}, Impl: "yes",
			},
		},
	}
}

func runKeys(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-KP-103":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{KeyNames: []string{rt.Name("missing")}})
			return wrapErr("runKeys", err)
		})
		return o, wrapErr("runKeys", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
