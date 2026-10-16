package ec2

import (
	"context"
	"fmt"
	"time"

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
		id, err := goneVolume(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				err := rt.EC2CreateTags(ctx, []string{id}, []types.Tag{
					{Key: aws.String("Name"), Value: aws.String(rt.Name("tag"))},
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
				err := rt.EC2CreateTags(ctx, []string{id}, rt.Tags())
				return wrapErr("runTags", err)
			})
			return callOut, wrapErr("runTags", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func goneVolume(ctx context.Context, rt *harness.Runtime, api *awsec2.Client) (string, error) {
	in := &awsec2.CreateVolumeInput{
		AvailabilityZone: aws.String("vws-1a"), Size: aws.Int32(harness.VolumeGiB),
		VolumeType: types.VolumeTypeGp2, TagSpecifications: rt.TagSpec(types.ResourceTypeVolume),
	}
	out, err := rt.EC2CreateVolume(ctx, in)
	if harness.UnsupportedParameter(err) {
		bare := *in
		bare.TagSpecifications = nil
		out, err = rt.EC2CreateVolume(ctx, &bare)
	}
	if err != nil {
		return "", fmt.Errorf("create volume: %w", err)
	}
	id := aws.ToString(out.VolumeId)
	rt.HoldEC2(api)
	rt.Cleanup(func(ctx context.Context) error {
		return rt.DeleteOwnedVolume(ctx, id)
	})
	if err := rt.Track("volume", id, rt.Name("tagmiss")); err != nil {
		return "", wrapErr("goneVolume", err)
	}
	if err := waitVolume(ctx, api, id); err != nil {
		return "", err
	}
	if err := rt.EC2DeleteVolume(ctx, id); err != nil {
		return "", fmt.Errorf("delete volume %s: %w", id, err)
	}
	return id, nil
}

func waitVolume(ctx context.Context, api *awsec2.Client, id string) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		out, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
		if err != nil {
			return fmt.Errorf("describe %s: %w", id, err)
		}
		if len(out.Volumes) > 0 {
			switch out.Volumes[0].State {
			case types.VolumeStateAvailable, types.VolumeStateInUse:
				return nil
			case types.VolumeStateError, types.VolumeStateDeleted, types.VolumeStateDeleting:
				return fmt.Errorf("volume %s is %s", id, out.Volumes[0].State)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("volume %s stayed creating: %w", id, ctx.Err())
		case <-tick.C:
		}
	}
}
