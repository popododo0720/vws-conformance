package ebs

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Volumes() bind.Group {
	return bind.Group{Service: "ebs", Name: "volumes", Run: runVol, Cases: []cases.Case{
		{
			ID: "EBS-VOL-104", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
			Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes",
		},
	}}
}

func runVol(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-VOL-104":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{
				VolumeIds: []string{"vol-0123456789abcdef0"},
			})
			return wrapErr("runVol", err)
		})
		return o, wrapErr("runVol", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
