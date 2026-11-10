package ebs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Volumes() bind.Group {
	return bind.Group{Service: "ebs", Name: "volumes", Run: runVol, Cases: []cases.Case{
		{
			ID: "EBS-VOL-004", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindHappy,
			Suite:  cases.SuiteSDK,
			DocURL: cases.DocEC2("ModifyVolume"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-VOL-106", Service: "ebs", Group: "volumes", Op: "ModifyVolume", Kind: cases.KindError,
			Suite:  cases.SuiteSDK,
			DocURL: cases.DocEC2("ModifyVolume"), Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue"},
			Impl: "yes",
		},
		{
			ID: "EBS-VOL-104", Service: "ebs", Group: "volumes", Op: "DescribeVolumes", Kind: cases.KindError,
			Suite:  cases.SuiteSDK,
			DocURL: cases.DocEC2Errors,
			Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}, Impl: "yes",
		},
	}}
}

func runVol(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	if c.ID == "EBS-VOL-004" || c.ID == "EBS-VOL-106" {
		vol, err := rt.EC2CreateVolume(ctx, &awsec2.CreateVolumeInput{
			AvailabilityZone: aws.String(rt.Cfg.Region + "a"), Size: aws.Int32(harness.VolumeGiB),
		})
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("modify setup: %w", err)
		}
		id := aws.ToString(vol.VolumeId)
		if vol.State != types.VolumeStateAvailable {
			query := &awsec2.DescribeVolumesInput{VolumeIds: []string{id}}
			if err := awsec2.NewVolumeAvailableWaiter(api).Wait(ctx, query, time.Minute); err != nil {
				return cases.Outcome{}, fmt.Errorf("modify volume ready: %w", err)
			}
		}
		target := harness.VolumeGiB + 1
		if c.ID == "EBS-VOL-106" {
			target = 0
		}
		out, err := rt.Call(ctx, func(ctx context.Context) error {
			modified, err := rt.EC2ModifyVolume(ctx, &awsec2.ModifyVolumeInput{
				VolumeId: aws.String(id), Size: aws.Int32(target),
			})
			if err != nil {
				return wrapErr("ModifyVolume", err)
			}
			m := modified.VolumeModification
			if m == nil || aws.ToString(m.VolumeId) != id || aws.ToInt32(m.TargetSize) != target ||
				aws.ToInt32(m.OriginalSize) != harness.VolumeGiB {
				return fmt.Errorf("ModifyVolume returned different values")
			}
			return nil
		})
		return out, wrapErr("runVol", err)
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
