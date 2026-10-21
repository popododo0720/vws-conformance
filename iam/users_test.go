package iam_test

import (
	"os"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/iam"
	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }

func TestUsers(t *testing.T) { engine.TestGroup(t, "iam", "users") }

func TestIAMSkippedUntilDeploy310(t *testing.T) {
	if len(iam.Cases()) < 10 {
		t.Fatalf("cases %d", len(iam.Cases()))
	}
	for _, c := range iam.Cases() {
		if !strings.Contains(c.Skip, "vws-deploy#310") {
			t.Fatalf("%s skip %q", c.ID, c.Skip)
		}
	}
}
