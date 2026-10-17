package ebs_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

func TestVolumes(t *testing.T)   { engine.TestGroup(t, "ebs", "volumes") }
func TestSnapshots(t *testing.T) { engine.TestGroup(t, "ebs", "snapshots") }
func TestGaps(t *testing.T)      { engine.TestGroup(t, "ebs", "gaps") }
