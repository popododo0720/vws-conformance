package ec2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Tags() bind.Group {
	return bind.Group{Service: "ec2", Name: "tags", Run: runTags, Cases: []cases.Case{
		{
			ID: "EC2-TAG-001", Service: "ec2", Group: "tags", Op: "CreateTags", Kind: cases.KindTags, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2("CreateTags"),
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"setup": "^ok$", "tag_state": "^present$"}}, Impl: "yes",
		},
		{
			ID: "EC2-TAG-101", Service: "ec2", Group: "tags", Op: "CreateTags", Kind: cases.KindError, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2Errors, Impl: "yes",
			Expect: cases.Expect{HTTP: 400, Code: "InvalidParameterValue", Patterns: map[string]string{"setup": "^ok$"}},
		},
		{
			ID: "EC2-GAP-007", Service: "ec2", Group: "tags", Op: "DeleteTags", Kind: cases.KindGap, Suite: cases.SuiteSDK,
			DocURL: cases.DocEC2("DeleteTags"),
			Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"setup": "^ok$", "tag_state": "^absent$"}}, Impl: "yes",
		},
	}}
}

func runTags(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	switch c.ID {
	case "EC2-TAG-001", "EC2-TAG-101", "EC2-GAP-007":
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	var volume *awsec2.CreateVolumeOutput
	setup, err := rt.Call(ctx, func(ctx context.Context) error {
		var createErr error
		volume, createErr = rt.EC2CreateVolume(ctx, &awsec2.CreateVolumeInput{
			AvailabilityZone: aws.String("vws-1a"), Size: aws.Int32(harness.VolumeGiB), VolumeType: types.VolumeTypeGp2,
		})
		return wrapErr("create volume", createErr)
	})
	if err != nil {
		return setup, fmt.Errorf("tag volume: %w", err)
	}
	if setup.Code != "" {
		setup.Fields["setup"] = "refused"
		return setup, nil
	}
	id := aws.ToString(volume.VolumeId)
	tags := []types.Tag{{Key: aws.String("Name"), Value: aws.String(rt.Name("tag"))}}
	if c.ID == "EC2-TAG-101" {
		tags[0].Value = aws.String(strings.Repeat("x", 257))
	}
	if c.ID == "EC2-GAP-007" {
		if _, err := rt.EC2CreateTags(ctx, id, tags); err != nil {
			return cases.Outcome{}, fmt.Errorf("seed tag: %w", err)
		}
	}
	outcome, err := rt.Call(ctx, func(ctx context.Context) error {
		if c.ID == "EC2-GAP-007" {
			_, err := rt.EC2DeleteTags(ctx, id, []types.Tag{{Key: aws.String("Name")}})
			return wrapErr("delete tag", err)
		}
		_, err := rt.EC2CreateTags(ctx, id, tags)
		return wrapErr("create tag", err)
	})
	if err != nil {
		return outcome, wrapErr("runTags", err)
	}
	outcome.Fields["setup"] = "ok"
	if c.ID == "EC2-TAG-101" {
		return outcome, nil
	}

	wanted := "present"
	if c.ID == "EC2-GAP-007" {
		wanted = "absent"
	}
	verifyCtx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		described, err := api.DescribeVolumes(verifyCtx, &awsec2.DescribeVolumesInput{VolumeIds: []string{id}})
		if err != nil {
			return outcome, fmt.Errorf("verify tags: %w", err)
		}
		if len(described.Volumes) != 1 || aws.ToString(described.Volumes[0].VolumeId) != id {
			return outcome, fmt.Errorf("verify tags: expected one exact volume")
		}
		owner := ""
		state := "absent"
		for _, tag := range described.Volumes[0].Tags {
			if aws.ToString(tag.Key) == harness.RunTagKey {
				owner = aws.ToString(tag.Value)
			}
			if aws.ToString(tag.Key) == "Name" {
				state = "present"
				if c.ID == "EC2-TAG-001" && aws.ToString(tag.Value) != rt.Name("tag") {
					state = "wrong-value"
				}
			}
		}
		if owner != rt.RunID() {
			return outcome, fmt.Errorf("tag mutation changed cleanup ownership")
		}
		if state == wanted {
			outcome.Fields["tag_state"] = state
			return outcome, nil
		}
		select {
		case <-verifyCtx.Done():
			return outcome, fmt.Errorf("wait for tag state %s: %w", wanted, verifyCtx.Err())
		case <-tick.C:
		}
	}
}
