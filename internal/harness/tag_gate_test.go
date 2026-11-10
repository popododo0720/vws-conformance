package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestTagGatePreservesOwnership(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EC2-TAG-001")
	id := createVolume(t, view)
	reserved := []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}}
	if _, err := view.EC2CreateTags(context.Background(), id, reserved); err == nil {
		t.Fatal("reserved create accepted")
	}
	if _, err := view.EC2DeleteTags(context.Background(), id, reserved); err == nil {
		t.Fatal("reserved delete accepted")
	}
	if _, err := view.EC2DeleteTags(context.Background(), id, nil); err == nil {
		t.Fatal("delete-all accepted")
	}
	if _, err := view.EC2DeleteTags(context.Background(), id, []types.Tag{{}}); err == nil {
		t.Fatal("empty-key delete accepted")
	}
	if fake.Calls("CreateTags") != 0 || fake.Calls("DeleteTags") != 0 {
		t.Fatal("ownership mutation reached SDK")
	}
	tags := []types.Tag{{Key: aws.String("Name"), Value: aws.String("example")}}
	for i := 0; i < 2; i++ {
		if _, err := view.EC2CreateTags(context.Background(), id, tags); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := fake.Volume(id); got["Name"] != "example" || got[RunTagKey] != testRun {
		t.Fatalf("tags = %v", got)
	}
	if _, err := view.EC2DeleteTags(context.Background(), id, []types.Tag{{Key: aws.String("Name")}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := fake.Volume(id); got[RunTagKey] != testRun || got["Name"] != "" {
		t.Fatalf("tags after delete = %v", got)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Volume(id); ok {
		t.Fatal("tag mutation lost cleanup registration")
	}
	if _, err := view.EC2CreateTags(context.Background(), id, tags); err == nil {
		t.Fatal("closed scope accepted tag write")
	}
}

func TestTagGateRefusesForeignVolume(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EC2-TAG-001")
	fake.PutVolume("vol-foreign", map[string]string{RunTagKey: foreignRun})
	tags := []types.Tag{{Key: aws.String("Name"), Value: aws.String("example")}}
	_, err := view.EC2CreateTags(context.Background(), "vol-foreign", tags)
	wantPreserve(t, err, "")
	_, err = view.EC2DeleteTags(context.Background(), "vol-foreign", tags)
	wantPreserve(t, err, "")
	if fake.Calls("CreateTags") != 0 || fake.Calls("DeleteTags") != 0 {
		t.Fatal("foreign tag mutation reached SDK")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}
