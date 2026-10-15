package ebs_test

import (
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/popododo0720/vws-conformance/ebs"
	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }

func TestVolumes(t *testing.T)   { engine.TestGroup(t, "ebs", "volumes") }
func TestSnapshots(t *testing.T) { engine.TestGroup(t, "ebs", "snapshots") }
func TestGaps(t *testing.T)      { engine.TestGroup(t, "ebs", "gaps") }

func TestVolumeUsesOneGiB(t *testing.T) {
	in := ebs.VolumeRequest("vws-1a", harness.VolumeGiB)
	if aws.ToInt32(in.Size) != 1 {
		t.Fatalf("size %d", aws.ToInt32(in.Size))
	}
	if aws.ToString(in.AvailabilityZone) != "vws-1a" {
		t.Fatal(aws.ToString(in.AvailabilityZone))
	}
}
