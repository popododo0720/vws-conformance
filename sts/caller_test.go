package sts_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/sts"
)

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }

func TestCaller(t *testing.T) { engine.TestGroup(t, "sts", "caller") }

func TestSTSSkippedUntilDeploy310(t *testing.T) {
	if !sts.Skipped() {
		t.Fatal("sts cases must stay skipped until vws-deploy#310")
	}
}
