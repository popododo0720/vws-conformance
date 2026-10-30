package s3

import (
	"regexp"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestBucketOwnedPatternMatchesRunScopedName(t *testing.T) {
	re := regexp.MustCompile(bucketOwnedPattern)
	name := harness.ResourceName("conf-20261030T021748Z-d7c604", "bkt-104")
	if !re.MatchString(name) {
		t.Fatalf("pattern does not match %q", name)
	}
	if re.MatchString(harness.ResourceName("conf-20261030T021748Z-d7c604", "bkt-105")) {
		t.Fatal("pattern matches another case name")
	}
}
