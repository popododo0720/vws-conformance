package ebs

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{Service: "ebs", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		{
			ID: "EBS-GAP-002", Service: "ebs", Group: "gaps", Op: "DescribeVolumesModifications", Kind: cases.KindGap,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVolumesModifications"),
			Expect: shapeExpect, Impl: "gap",
		},
	}}
}

// shapeExpect is the AWS response-shape expectation for the read-only gap cases.
var shapeExpect = cases.Expect{HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern}}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-GAP-002":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeVolumesModifications(ctx, &awsec2.DescribeVolumesModificationsInput{})
			return wrapErr("runGaps", err)
		})
		o.Fields["shape"] = expect.ShapeField(
			o, "DescribeVolumesModifications", "volumeModificationSet",
			"volumeId", "modificationState",
		)
		return o, wrapErr("runGaps", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
