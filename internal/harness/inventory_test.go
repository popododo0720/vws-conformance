package harness

import (
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

func TestEC2InventoryIncludesDryRunKinds(t *testing.T) {
	run := "conf-20261015T120000Z-abc123"
	rows := []sweep.Resource{{Kind: "address", ID: "eipalloc-1", LedgerRunID: run}}
	tag := []ec2types.Tag{{Key: aws.String("vws:conformance-run"), Value: aws.String(run)}}
	view := ec2View{
		addresses: []ec2types.Address{{AllocationId: aws.String("eipalloc-1"), AssociationId: aws.String("eipassoc-1")}},
		groups:    []ec2types.SecurityGroup{{GroupId: aws.String("sg-1"), GroupName: aws.String("conf-sg")}},
		subnets: []ec2types.Subnet{
			{SubnetId: aws.String("subnet-1"), Tags: tag},
			{SubnetId: aws.String("subnet-foreign")},
		},
		vpcs:     []ec2types.Vpc{{VpcId: aws.String("vpc-1"), Tags: []ec2types.Tag{{Key: aws.String("Name"), Value: aws.String("conf-vpc")}}}},
		gateways: []ec2types.InternetGateway{{InternetGatewayId: aws.String("igw-1"), Tags: tag, Attachments: []ec2types.InternetGatewayAttachment{{VpcId: aws.String("vpc-1")}}}},
	}
	got := map[string]bool{}
	for _, item := range ec2Items(view, rows) {
		got[item.kind+" "+item.id] = true
	}
	for _, want := range []string{"address eipalloc-1", "security-group sg-1", "subnet subnet-1", "vpc vpc-1", "internet-gateway igw-1"} {
		if !got[want] {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
	if got["subnet subnet-foreign"] {
		t.Fatal("untagged subnet was selected")
	}
}

func TestTerminatedInstanceIsNotALeftover(t *testing.T) {
	run := "conf-20261015T120000Z-abc123"
	tag := []ec2types.Tag{{Key: aws.String("vws:conformance-run"), Value: aws.String(run)}}
	view := ec2View{instances: []ec2types.Instance{
		{
			InstanceId: aws.String("i-gone"),
			State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated},
			Tags:       tag,
		},
		{
			InstanceId: aws.String("i-down"),
			State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNameShuttingDown},
			Tags:       tag,
		},
	}}
	got := map[string]bool{}
	for _, item := range ec2Items(view, nil) {
		got[item.kind+" "+item.id] = true
	}
	if got["instance i-gone"] {
		t.Fatal("terminated instance was a leftover")
	}
	if !got["instance i-down"] {
		t.Fatal("shutting-down instance was ignored")
	}
}

func TestIgnoreCleanup(t *testing.T) {
	for _, msg := range []string{
		"api error InvalidAllocationID.NotFound: not found",
		"api error DependencyViolation: subnet has dependents",
		"api error IncorrectState: volume is in use",
	} {
		if err := IgnoreCleanup(errors.New(msg)); err != nil {
			t.Fatal(err)
		}
	}
	if err := IgnoreCleanup(errors.New("api error UnauthorizedOperation: denied")); err == nil {
		t.Fatal("auth error was ignored")
	}
}

func TestUnsupportedAction(t *testing.T) {
	if !unsupportedAction(errors.New("api error InvalidAction: DescribeInternetGateways")) {
		t.Fatal("InvalidAction was fatal")
	}
	if unsupportedAction(errors.New("deadline exceeded")) {
		t.Fatal("deadline was treated as unsupported")
	}
	if unsupportedAction(nil) {
		t.Fatal("nil error was unsupported")
	}
}
