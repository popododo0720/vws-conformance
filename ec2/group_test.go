package ec2_test

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/popododo0720/vws-conformance/ec2"
	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

func TestInstances(t *testing.T) { engine.TestGroup(t, "ec2", "instances") }
func TestImages(t *testing.T)    { engine.TestGroup(t, "ec2", "images") }
func TestKeyPairs(t *testing.T)  { engine.TestGroup(t, "ec2", "keypairs") }
func TestAddresses(t *testing.T) { engine.TestGroup(t, "ec2", "addresses") }
func TestTags(t *testing.T)      { engine.TestGroup(t, "ec2", "tags") }
func TestGaps(t *testing.T)      { engine.TestGroup(t, "ec2", "gaps") }

func TestLaunchUsesSmallestType(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123", Region: "vws-1"}, nil)
	in := ec2.LaunchRequest(rt, "subnet-0123456789abcdef0", "token1")
	if string(in.InstanceType) != "t2.nano" {
		t.Fatalf("type %s", in.InstanceType)
	}
	if aws.ToInt32(in.MinCount) != 1 || aws.ToInt32(in.MaxCount) != 1 {
		t.Fatalf("count %d %d", aws.ToInt32(in.MinCount), aws.ToInt32(in.MaxCount))
	}
	if aws.ToString(in.ClientToken) != "token1" {
		t.Fatal(aws.ToString(in.ClientToken))
	}
}
