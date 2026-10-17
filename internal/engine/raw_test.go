package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestRawDirectoryFailureIsHarnessError(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := runRaw(t, blocked, "conf-20261016T090000Z-aabbcc", "RAW-001")
	if res.Status != "error" {
		t.Fatalf("status %s reason %q", res.Status, res.Reason)
	}
}

func TestRawFileFailureIsHarnessError(t *testing.T) {
	dir := t.TempDir()
	const run = "conf-20261016T090100Z-bbccdd"
	const id = "RAW-002"
	rawDir := filepath.Join(dir, run, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(rawDir, id+".txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := runRaw(t, dir, run, id)
	if res.Status != "error" {
		t.Fatalf("status %s raw %q reason %q", res.Status, res.RawRef, res.Reason)
	}
}

func runRaw(t *testing.T, out, run, id string) (res struct{ Status, Reason, RawRef string }) {
	t.Helper()
	rt := harness.NewRuntime(harness.Config{RunID: run, OutDir: out}, nil)
	g := bind.Group{Run: func(context.Context, *harness.Runtime, cases.Case) (cases.Outcome, error) {
		return cases.Outcome{HTTP: 200, Raw: "body"}, nil
	}}
	got := runOne(context.Background(), rt, g, cases.Case{ID: id, Expect: cases.Expect{HTTP: 200}})
	return struct{ Status, Reason, RawRef string }{got.Status, got.Reason, got.RawRef}
}
