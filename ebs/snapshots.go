package ebs

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Snapshots() bind.Group {
	return bind.Group{Service: "ebs", Name: "snapshots", Run: runSnap, Cases: []cases.Case{
		{
			ID:      "EBS-SNAP-001",
			Service: "ebs", Group: "snapshots", Op: "CreateSnapshot", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateSnapshot"),
			Expect: cases.Expect{HTTP: 200},
			Impl:   "yes",
		},
		{
			ID:      "EBS-SNAP-002",
			Service: "ebs", Group: "snapshots", Op: "DescribeSnapshots", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeSnapshots"),
			Expect: cases.Expect{HTTP: 200},
			Impl:   "yes",
		},
		{
			ID:      "EBS-SNAP-003",
			Service: "ebs", Group: "snapshots", Op: "DeleteSnapshot", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteSnapshot"),
			Expect: cases.Expect{HTTP: 200},
			Impl:   "yes",
		},
		{
			ID:      "EBS-SNAP-101",
			Service: "ebs", Group: "snapshots", Op: "DeleteSnapshot", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors,
			Expect: cases.Expect{HTTP: 400, Code: "InvalidSnapshot.NotFound"},
			Impl:   "yes",
		},
		{
			ID:      "EBS-SNAP-103",
			Service: "ebs", Group: "snapshots", Op: "CreateSnapshot", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateSnapshot"),
			Expect: cases.Expect{HTTP: 400, Code: "MissingParameter"},
			Impl:   "yes",
		},

		{
			ID: "EBS-SNAP-102", Service: "ebs", Group: "snapshots", Op: "DescribeSnapshots", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
				HTTP: 400,
				Code: "InvalidSnapshot.NotFound",
			}, Impl: "yes",
		},
	}}
}

func runSnap(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	var volumeID, snapshotID string
	switch c.ID {
	case "EBS-SNAP-001", "EBS-SNAP-002", "EBS-SNAP-003", "EBS-SNAP-101":
		vol, err := rt.EC2CreateVolume(ctx, &awsec2.CreateVolumeInput{
			AvailabilityZone: aws.String(rt.Cfg.Region + "a"), Size: aws.Int32(harness.VolumeGiB),
		})
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("snapshot source: %w", err)
		}
		volumeID = aws.ToString(vol.VolumeId)
		query := &awsec2.DescribeVolumesInput{VolumeIds: []string{volumeID}}
		if err := awsec2.NewVolumeAvailableWaiter(api).Wait(ctx, query, time.Minute); err != nil {
			return cases.Outcome{}, fmt.Errorf("snapshot source ready: %w", err)
		}
		if c.ID != "EBS-SNAP-001" {
			snap, err := rt.EC2CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{VolumeId: aws.String(volumeID)})
			if err != nil {
				return cases.Outcome{}, fmt.Errorf("snapshot setup: %w", err)
			}
			snapshotID = aws.ToString(snap.SnapshotId)
		}
		if c.ID == "EBS-SNAP-101" {
			if _, err := rt.EC2DeleteSnapshot(ctx, snapshotID); err != nil {
				return cases.Outcome{}, fmt.Errorf("snapshot first delete: %w", err)
			}
		}
	}
	var call func(context.Context) error
	switch c.ID {
	case "EBS-SNAP-001", "EBS-SNAP-103":
		call = func(ctx context.Context) error {
			out, err := rt.EC2CreateSnapshot(ctx, &awsec2.CreateSnapshotInput{VolumeId: aws.String(volumeID)})
			if err == nil && aws.ToString(out.SnapshotId) == "" {
				return fmt.Errorf("CreateSnapshot returned no snapshot id")
			}
			return wrapErr("CreateSnapshot", err)
		}
	case "EBS-SNAP-002":
		call = func(ctx context.Context) error {
			out, err := api.DescribeSnapshots(ctx, &awsec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}})
			if err != nil {
				return wrapErr("DescribeSnapshots", err)
			}
			if len(out.Snapshots) != 1 || aws.ToString(out.Snapshots[0].SnapshotId) != snapshotID ||
				aws.ToString(out.Snapshots[0].VolumeId) != volumeID {
				return fmt.Errorf("DescribeSnapshots returned a different snapshot or source")
			}
			return nil
		}
	case "EBS-SNAP-003", "EBS-SNAP-101":
		call = func(ctx context.Context) error {
			_, err := rt.EC2DeleteSnapshot(ctx, snapshotID)
			return wrapErr("DeleteSnapshot", err)
		}
	case "EBS-SNAP-102":
		call = func(ctx context.Context) error {
			_, err := api.DescribeSnapshots(ctx, &awsec2.DescribeSnapshotsInput{
				SnapshotIds: []string{"snap-0123456789abcdef0"},
			})
			return wrapErr("DescribeSnapshots", err)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
	out, err := rt.Call(ctx, call)
	return out, wrapErr("runSnap", err)
}
