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

func Tags() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "tags", Run: runTags,
		Cases: []cases.Case{
			{
				ID: "EC2-TAG-001", Service: "ec2", Group: "tags", Op: "CreateTags", Kind: cases.KindTags, Suite: cases.SuiteSDK,
				DocURL: cases.DocEC2("CreateTags"), Expect: cases.Expect{HTTP: 200}, Impl: "yes", Slow: true,
			},
			{
				ID: "EC2-TAG-101", Service: "ec2", Group: "tags", Op: "CreateTags", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue"}, Impl: "yes",
			},
		},
	}
}

func runTags(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EC2-TAG-101":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{"vol-0123456789abcdef0"},
					Tags:      []types.Tag{{Key: aws.String("Name"), Value: aws.String(rt.Name("tag"))}},
				})
				return wrapErr("runTags", err)
			})
			return callOut, wrapErr("runTags", callErr)
		}
	case "EC2-TAG-001":
		_, subnet, err := rt.EnsureNet(ctx)
		if err != nil {
			return cases.Outcome{}, wrapErr("runTags", err)
		}
		id, err := launch(ctx, rt, api, subnet, rt.Token("tag"))
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateTags(ctx, &awsec2.CreateTagsInput{
					Resources: []string{id},
					Tags:      rt.Tags(),
				})
				return wrapErr("runTags", err)
			})
			return callOut, wrapErr("runTags", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
