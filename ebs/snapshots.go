package ebs

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Snapshots() bind.Group {
	return bind.Group{Service: "ebs", Name: "snapshots", Run: runSnap, Cases: []cases.Case{
		{
			ID: "EBS-SNAP-001", Service: "ebs", Group: "snapshots", Op: "CreateSnapshot", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateSnapshot"), Expect: cases.Expect{
				HTTP:     200,
				Patterns: map[string]string{"id": expect.SnapID},
			}, Impl: "yes",
		},
		{
			ID: "EBS-SNAP-002", Service: "ebs", Group: "snapshots", Op: "DescribeSnapshots", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeSnapshots"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-SNAP-003", Service: "ebs", Group: "snapshots", Op: "DeleteSnapshot", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteSnapshot"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
		},
		{
			ID: "EBS-SNAP-101", Service: "ebs", Group: "snapshots", Op: "DeleteSnapshot", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidSnapshot.NotFound",
			}, Impl: "yes",
		},
		{
			ID: "EBS-SNAP-102", Service: "ebs", Group: "snapshots", Op: "DescribeSnapshots", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidSnapshot.NotFound",
			}, Impl: "yes",
		},
		{
			ID: "EBS-SNAP-103", Service: "ebs", Group: "snapshots", Op: "CreateSnapshot", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"},
			Impl: "yes",
		},
	}}
}

func runSnap(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	switch c.ID {
	case "EBS-SNAP-101", "EBS-SNAP-102":
		id, err := goneSnap(ctx, rt, api)
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				var err error
				if c.ID == "EBS-SNAP-102" {
					_, err = api.DescribeSnapshots(ctx, &awsec2.DescribeSnapshotsInput{SnapshotIds: []string{id}})
				} else {
					_, err = api.DeleteSnapshot(ctx, &awsec2.DeleteSnapshotInput{SnapshotId: aws.String(id)})
				}
				return wrapErr("runSnap", err)
			})
			return callOut, wrapErr("runSnap", callErr)
		}
	case "EBS-SNAP-103":
		vol, err := goneVol(ctx, rt, api, "snapmiss")
		if err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{
					VolumeId:    aws.String(vol),
					Description: aws.String(rt.Name("missing")),
				})
				return wrapErr("runSnap", err)
			})
			return callOut, wrapErr("runSnap", callErr)
		}
	}
	vol, err := makeVol(ctx, rt, api, "snapvol")
	if err != nil {
		return cases.Outcome{}, err
	}
	if _, err := api.DescribeVolumes(ctx, &awsec2.DescribeVolumesInput{VolumeIds: []string{vol}}); err != nil {
		return cases.Outcome{}, fmt.Errorf("describe volume: %w", err)
	}
	desc := rt.Name("snap")
	switch c.ID {
	case "EBS-SNAP-001":
		var id string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{
				VolumeId:    aws.String(vol),
				Description: aws.String(desc),
			})
			if err == nil {
				id = aws.ToString(out.SnapshotId)
			}
			return wrapErr("runSnap", err)
		})
		o.Fields["id"] = id
		return o, wrapErr("runSnap", err)
	case "EBS-SNAP-002", "EBS-SNAP-003":
		out, err := api.CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{
			VolumeId:    aws.String(vol),
			Description: aws.String(desc),
		})
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("snapshot: %w", err)
		}
		id := aws.ToString(out.SnapshotId)
		if err := rt.Track("snapshot", id, desc); err != nil {
			return cases.Outcome{}, wrapErr("runSnap", err)
		}
		if c.ID == "EBS-SNAP-002" {
			{
				callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
					_, err := api.DescribeSnapshots(ctx, &awsec2.DescribeSnapshotsInput{
						SnapshotIds: []string{id},
						OwnerIds:    []string{"self"},
					})
					return wrapErr("runSnap", err)
				})
				return callOut, wrapErr("runSnap", callErr)
			}
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteSnapshot(ctx, &awsec2.DeleteSnapshotInput{SnapshotId: aws.String(id)})
				return wrapErr("runSnap", err)
			})
			return callOut, wrapErr("runSnap", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func goneSnap(ctx context.Context, rt *harness.Runtime, api *awsec2.Client) (string, error) {
	vol, err := makeVol(ctx, rt, api, "snapgone")
	if err != nil {
		return "", err
	}
	desc := rt.Name("snapgone")
	out, err := api.CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{
		VolumeId: aws.String(vol), Description: aws.String(desc),
	})
	if err != nil {
		return "", fmt.Errorf("snapshot: %w", err)
	}
	id := aws.ToString(out.SnapshotId)
	if err := rt.Track("snapshot", id, desc); err != nil {
		return "", wrapErr("goneSnap", err)
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := api.DeleteSnapshot(ctx, &awsec2.DeleteSnapshotInput{SnapshotId: aws.String(id)})
		return harness.IgnoreCleanup(err)
	})
	if _, err := api.DeleteSnapshot(ctx, &awsec2.DeleteSnapshotInput{SnapshotId: aws.String(id)}); err != nil {
		return "", fmt.Errorf("delete snapshot %s: %w", id, err)
	}
	return id, nil
}
