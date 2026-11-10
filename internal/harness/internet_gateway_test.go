package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/popododo0720/vws-conformance/internal/ec2fake"
)

func TestInternetGatewayCreateAndCleanup(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-GAP-006")
	input := &ec2.CreateInternetGatewayInput{}
	input.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeInternetGateway,
		Tags:         []types.Tag{{Key: aws.String("Name"), Value: aws.String("test")}},
	}}
	out, err := view.EC2CreateInternetGateway(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	id := aws.ToString(out.InternetGateway.InternetGatewayId)
	tags, ok := fake.InternetGateway(id)
	if !ok || tags[RunTagKey] != testRun || tags["Name"] != "test" {
		t.Fatalf("tags %v", tags)
	}
	if len(input.TagSpecifications[0].Tags) != 1 {
		t.Fatal("caller input mutated")
	}
	for i := 0; i < 2; i++ {
		if err := view.EndCase(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := fake.InternetGateway(id); ok || fake.Calls("DeleteInternetGateway") != 1 {
		t.Fatalf("present %v deletes %d", ok, fake.Calls("DeleteInternetGateway"))
	}
	if fake.Calls("DescribeInternetGateways") == 0 {
		t.Fatal("cleanup did not observe ownership")
	}
}

func TestInternetGatewayForeignOwnerIsPreserved(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-GAP-006")
	out, err := view.EC2CreateInternetGateway(context.Background(), &ec2.CreateInternetGatewayInput{})
	if err != nil {
		t.Fatal(err)
	}
	id := aws.ToString(out.InternetGateway.InternetGatewayId)
	fake.PutInternetGateway(id, map[string]string{RunTagKey: foreignRun})
	_, err = view.EC2DeleteInternetGateway(context.Background(), id)
	wantPreserve(t, err, "foreign owner")
	wantPreserve(t, view.EndCase(context.Background()), "foreign owner")
	if fake.Calls("DeleteInternetGateway") != 0 {
		t.Fatal("foreign resource was deleted")
	}
}

func TestInternetGatewayRejectsForgedTagsAndNilInput(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-GAP-006")
	input := &ec2.CreateInternetGatewayInput{}
	input.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeInternetGateway,
		Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
	}}
	if _, err := view.EC2CreateInternetGateway(context.Background(), input); err == nil {
		t.Fatal("forged run tag accepted")
	}
	if _, err := view.EC2CreateInternetGateway(context.Background(), nil); err == nil {
		t.Fatal("nil input accepted")
	}
	if fake.Calls("CreateInternetGateway") != 0 {
		t.Fatal("rejected request was sent")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInternetGatewayLostResponsePreservesUnknownOutcome(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-GAP-006")
	fake.SetCreateHook(func() ec2fake.CreateResult { return ec2fake.CreateResult{Lose: true} })
	if _, err := view.EC2CreateInternetGateway(context.Background(), &ec2.CreateInternetGatewayInput{}); err == nil {
		t.Fatal("lost response succeeded")
	}
	wantPreserve(t, view.EndCase(context.Background()), "create outcome unknown")
	if fake.Calls("CreateInternetGateway") != 1 || fake.Calls("DeleteInternetGateway") != 0 {
		t.Fatalf("create %d delete %d", fake.Calls("CreateInternetGateway"), fake.Calls("DeleteInternetGateway"))
	}
}

func TestInternetGatewayRefusesNeverOwnedID(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-GAP-006")
	_, err := view.EC2DeleteInternetGateway(context.Background(), "igw-0123456789abcdef0")
	wantPreserve(t, err, "never ours")
	if fake.Calls("DeleteInternetGateway") != 0 {
		t.Fatal("unowned delete was sent")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}
