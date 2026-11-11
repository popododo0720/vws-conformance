package engine

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestVolumeAssertionFailureCleansUp(t *testing.T) {
	fake, rt := gateRuntime(t)
	var id string
	g := bind.Group{Run: func(ctx context.Context, crt *harness.Runtime, _ cases.Case) (cases.Outcome, error) {
		return crt.Call(ctx, func(ctx context.Context) error {
			out, err := crt.EC2CreateVolume(ctx, &ec2.CreateVolumeInput{
				AvailabilityZone: aws.String("vws-1a"), Size: aws.Int32(harness.VolumeGiB),
			})
			if err == nil {
				id = aws.ToString(out.VolumeId)
			}
			return wrapErr("create volume", err)
		})
	}}
	res := runOne(context.Background(), rt, g, cases.Case{ID: "EBS-VOL-001", Expect: cases.Expect{HTTP: 201}})
	if res.Status != "fail" {
		t.Fatalf("expected assertion failure, got %+v", res)
	}
	if _, ok := fake.Volume(id); id == "" || ok {
		t.Fatalf("failed case left volume %q", id)
	}
}
