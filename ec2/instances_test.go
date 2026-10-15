package ec2

import (
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestDescribedInstanceUsesResponse(t *testing.T) {
	launched := "i-0123456789abcdef0"
	out := &awsec2.DescribeInstancesOutput{Reservations: []types.Reservation{{
		Instances: []types.Instance{{InstanceId: aws.String(launched)}},
	}}}
	got, err := describedInstance(out, launched, 200)
	if err != nil || got != launched {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err = describedInstance(out, "i-0000000000000001", 200); err == nil || !strings.Contains(err.Error(), "returned") {
		t.Fatalf("mismatch err %v", err)
	}
	if _, err = describedInstance(&awsec2.DescribeInstancesOutput{}, launched, 200); err == nil {
		t.Fatal("empty success was accepted")
	}
	got, err = describedInstance(out, launched, 400)
	if err != nil || got != "" {
		t.Fatalf("http error became %q %v", got, err)
	}
}
