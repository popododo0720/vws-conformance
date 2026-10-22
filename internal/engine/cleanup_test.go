package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/report"
)

func TestCleanupErrorIsHarnessError(t *testing.T) {
	dir := t.TempDir()
	rep := report.New("conf-20261015T120000Z-abc123", report.Target{
		Name: "local-fake", Region: "vws-1", ConsoleAPIVersion: "local",
	}, report.Tools{Go: "go1.26.9", SDK: "v1.47.3", CLI: "not-installed"})
	rep.Finish()
	cfg := harness.Config{RunID: rep.RunID, OutDir: dir, Target: "local-fake"}
	publishReport(rep, cfg, "report.json")
	if err := noteCleanup(errors.New("sweep failed")); err == nil {
		t.Fatal("expected error")
	}
	if rep.Summary.Error != 1 {
		t.Fatalf("summary %+v", rep.Summary)
	}
	if rep.Results[0].ID != "HARNESS-CLEANUP" || rep.Results[0].Status != "error" {
		t.Fatalf("%+v", rep.Results[0])
	}
	b, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join(repoRoot(t), "schema", "report.schema.json")
	if err := report.Validate(schema, b); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
