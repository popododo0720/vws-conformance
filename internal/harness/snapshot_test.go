package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestSnapshotGateTagsAndCleansUp(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-SNAP-001")
	volume := createVolume(t, view)
	in := &ec2.CreateSnapshotInput{
		VolumeId: aws.String(volume),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeSnapshot,
			Tags:         []types.Tag{{Key: aws.String("Name"), Value: aws.String("backup")}},
		}},
	}
	out, err := view.EC2CreateSnapshot(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	tags, ok := fake.Snapshot(aws.ToString(out.SnapshotId))
	if !ok || tags[RunTagKey] != testRun || tags["Name"] != "backup" {
		t.Fatalf("tags=%v present=%v", tags, ok)
	}
	if len(in.TagSpecifications[0].Tags) != 1 {
		t.Fatal("input mutated")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, s := fake.ResourceCounts(); v != 0 || s != 0 {
		t.Fatalf("left %d volumes %d snapshots", v, s)
	}
	if fake.Calls("DeleteSnapshot") != 1 {
		t.Fatal("cleanup repeated snapshot deletion")
	}
}

func TestSnapshotGatePreservesForeignAndRejectsForgedTags(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-SNAP-003")
	fake.PutSnapshot("snap-0123456789abcdef0", "vol-0123456789abcdef0", map[string]string{RunTagKey: foreignRun})
	_, err := view.EC2DeleteSnapshot(context.Background(), "snap-0123456789abcdef0")
	wantPreserve(t, err, "foreign owner")
	if fake.Calls("DeleteSnapshot") != 0 {
		t.Fatal("foreign snapshot deleted")
	}
	in := &ec2.CreateSnapshotInput{
		VolumeId: aws.String("unused"),
		TagSpecifications: []types.TagSpecification{{
			ResourceType: types.ResourceTypeSnapshot,
			Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
		}},
	}
	if _, err := view.EC2CreateSnapshot(context.Background(), in); err == nil {
		t.Fatal("accepted forged tag")
	}
	if _, err := view.EC2CreateSnapshot(context.Background(), nil); err == nil {
		t.Fatal("accepted nil input")
	}
	if fake.Calls("CreateSnapshot") != 0 {
		t.Fatal("sent invalid input")
	}
}

func TestSnapshotGateCleansAfterCancellation(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-SNAP-001")
	volume := createVolume(t, view)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := view.EC2CreateSnapshot(ctx, &ec2.CreateSnapshotInput{VolumeId: aws.String(volume)}); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, s := fake.ResourceCounts(); v != 0 || s != 0 {
		t.Fatalf("canceled case left %d volumes %d snapshots", v, s)
	}
}
