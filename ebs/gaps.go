package ebs

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{Service: "ebs", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		{
			ID: "EBS-GAP-002", Service: "ebs", Group: "gaps", Op: "DescribeVolumesModifications", Kind: cases.KindGap,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVolumesModifications"), Expect: cases.Expect{HTTP: 200},
			Impl: "gap",
		},
	}}
}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-GAP-002":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeVolumesModifications(ctx, &awsec2.DescribeVolumesModificationsInput{})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
