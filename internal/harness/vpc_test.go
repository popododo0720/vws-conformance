package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/popododo0720/vws-conformance/internal/ec2fake"
)

func TestVPCCreateAndCleanup(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-VPC-001")
	input := &ec2.CreateVpcInput{CidrBlock: aws.String("10.44.0.0/16")}
	input.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeVpc,
		Tags:         []types.Tag{{Key: aws.String("Name"), Value: aws.String("test")}},
	}}
	out, err := view.EC2CreateVpc(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	id := aws.ToString(out.Vpc.VpcId)
	tags, ok := fake.Vpc(id)
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
	if _, ok := fake.Vpc(id); ok || fake.Calls("DeleteVpc") != 1 {
		t.Fatalf("present %v deletes %d", ok, fake.Calls("DeleteVpc"))
	}
	if fake.Calls("DescribeVpcs") == 0 {
		t.Fatal("cleanup did not observe ownership")
	}
}

func TestVPCForeignOwnerIsPreserved(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-VPC-001")
	out, err := view.EC2CreateVpc(context.Background(), &ec2.CreateVpcInput{CidrBlock: aws.String("10.44.0.0/16")})
	if err != nil {
		t.Fatal(err)
	}
	id := aws.ToString(out.Vpc.VpcId)
	fake.PutVpc(id, map[string]string{RunTagKey: foreignRun})
	_, err = view.EC2DeleteVpc(context.Background(), id)
	wantPreserve(t, err, "foreign owner")
	wantPreserve(t, view.EndCase(context.Background()), "foreign owner")
	if fake.Calls("DeleteVpc") != 0 {
		t.Fatal("foreign resource was deleted")
	}
}

func TestVPCRejectsForgedTagsAndNilInput(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-VPC-001")
	input := &ec2.CreateVpcInput{CidrBlock: aws.String("10.44.0.0/16")}
	input.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeVpc,
		Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
	}}
	if _, err := view.EC2CreateVpc(context.Background(), input); err == nil {
		t.Fatal("forged run tag accepted")
	}
	if _, err := view.EC2CreateVpc(context.Background(), nil); err == nil {
		t.Fatal("nil input accepted")
	}
	if fake.Calls("CreateVpc") != 0 {
		t.Fatal("rejected request was sent")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestVPCLostResponsePreservesUnknownOutcome(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-VPC-001")
	fake.SetCreateHook(func() ec2fake.CreateResult { return ec2fake.CreateResult{Lose: true} })
	input := &ec2.CreateVpcInput{CidrBlock: aws.String("10.44.0.0/16")}
	if _, err := view.EC2CreateVpc(context.Background(), input); err == nil {
		t.Fatal("lost response succeeded")
	}
	wantPreserve(t, view.EndCase(context.Background()), "create outcome unknown")
	if fake.Calls("CreateVpc") != 1 || fake.Calls("DeleteVpc") != 0 {
		t.Fatalf("create %d delete %d", fake.Calls("CreateVpc"), fake.Calls("DeleteVpc"))
	}
}

func TestVPCRefusesNeverOwnedID(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "VPC-VPC-001")
	_, err := view.EC2DeleteVpc(context.Background(), "vpc-0123456789abcdef0")
	wantPreserve(t, err, "never ours")
	if fake.Calls("DeleteVpc") != 0 {
		t.Fatal("unowned delete was sent")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}
