package s3_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

func TestBuckets(t *testing.T)  { engine.TestGroup(t, "s3", "buckets") }
func TestObjects(t *testing.T)  { engine.TestGroup(t, "s3", "objects") }
func TestListings(t *testing.T) { engine.TestGroup(t, "s3", "listings") }
func TestGaps(t *testing.T)     { engine.TestGroup(t, "s3", "gaps") }
