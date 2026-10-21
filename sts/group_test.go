package sts_test

import (
	"os"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/engine"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}

func TestCaller(t *testing.T) { engine.TestGroup(t, "sts", "caller") }
