package ebs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{Service: "ebs", Name: "gaps", Run: runGaps, Cases: []cases.Case{
		{ID: "EBS-GAP-001", Service: "ebs", Group: "gaps", Op: "DetachVolume", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DetachVolume"), Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "gap"},
		{ID: "EBS-GAP-002", Service: "ebs", Group: "gaps", Op: "DescribeVolumesModifications", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeVolumesModifications"), Expect: cases.Expect{HTTP: 200}, Impl: "gap"},
	}}
}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-GAP-001":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DetachVolume(ctx, &awsec2.DetachVolumeInput{VolumeId: aws.String("vol-0123456789abcdef0")})
			return err
		})
	case "EBS-GAP-002":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeVolumesModifications(ctx, &awsec2.DescribeVolumesModificationsInput{})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
