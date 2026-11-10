package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func TestModifyVolumeGateOwnsTargetAndRetainsCleanup(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-004")
	id := createVolume(t, view)
	in := &ec2.ModifyVolumeInput{
		VolumeId: aws.String(id), Size: aws.Int32(2),
	}
	if _, err := view.EC2ModifyVolume(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if fake.VolumeSize(id) != 2 {
		t.Fatal("fake did not apply modification")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Volume(id); ok {
		t.Fatal("modified volume escaped cleanup")
	}
}

func TestModifyVolumeGatePreservesForeignAndAbsentTargets(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-106")
	id := "vol-0123456789abcdef0"
	fake.PutVolume(id, map[string]string{RunTagKey: foreignRun})
	_, err := view.EC2ModifyVolume(context.Background(), &ec2.ModifyVolumeInput{
		VolumeId: aws.String(id), Size: aws.Int32(2),
	})
	wantPreserve(t, err, "foreign owner")
	_, err = view.EC2ModifyVolume(context.Background(), &ec2.ModifyVolumeInput{
		VolumeId: aws.String("vol-00000000000000000"),
	})
	wantPreserve(t, err, "never ours")
	if _, err := view.EC2ModifyVolume(context.Background(), nil); err == nil {
		t.Fatal("accepted nil input")
	}
	if fake.Calls("ModifyVolume") != 0 {
		t.Fatal("unsafe modification was sent")
	}
}
