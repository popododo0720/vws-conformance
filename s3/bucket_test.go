package s3

import (
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestBucketNameIsUniquePerCase(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123"}, nil)
	a := bucketName(rt, "S3-OBJ-001")
	b := bucketName(rt, "S3-OBJ-002")
	c := bucketName(rt, "S3-LO-001")
	if a == b || a == c || b == c {
		t.Fatalf("%s %s %s", a, b, c)
	}
	for _, name := range []string{a, b, c} {
		if name != strings.ToLower(name) || !strings.HasPrefix(name, "conf-") {
			t.Fatal(name)
		}
	}
}
