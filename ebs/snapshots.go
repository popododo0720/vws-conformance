package ebs

import (
	"context"
	"fmt"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Snapshots() bind.Group {
	return bind.Group{Service: "ebs", Name: "snapshots", Run: runSnap, Cases: []cases.Case{
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
	switch c.ID {
	case "EBS-SNAP-102":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DescribeSnapshots(ctx, &awsec2.DescribeSnapshotsInput{
				SnapshotIds: []string{"snap-0123456789abcdef0"},
			})
			return wrapErr("runSnap", err)
		})
		return o, wrapErr("runSnap", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
