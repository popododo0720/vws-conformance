package ec2_test

import (
	"os"
	"testing"

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
func TestGaps(t *testing.T)      { engine.TestGroup(t, "ec2", "gaps") }
