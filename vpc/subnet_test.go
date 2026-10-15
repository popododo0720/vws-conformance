package vpc

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestSubnetInputCarriesRunID(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123"}, nil)
	in := subnetInput(rt, "vpc-1", "10.12.2.0/24")
	if !specHas(in.TagSpecifications, "vws:conformance-run", rt.RunID()) {
		t.Fatal("subnet input has no run id tag")
	}
}

func specHas(specs []types.TagSpecification, key, value string) bool {
	for _, spec := range specs {
		for _, tag := range spec.Tags {
			if aws.ToString(tag.Key) == key && aws.ToString(tag.Value) == value {
				return true
			}
		}
	}
	return false
}
