package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/report"
)

func TestRunOneRunsCaseCleanup(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, *harness.Runtime) (cases.Outcome, error)
		want string
	}{
		{
			name: "success",
			run: func(_ context.Context, _ *harness.Runtime) (cases.Outcome, error) {
				return cases.Outcome{HTTP: 200}, nil
			},
			want: "pass",
		},
		{
			name: "error",
			run: func(_ context.Context, _ *harness.Runtime) (cases.Outcome, error) {
				return cases.Outcome{}, errors.New("boom")
			},
			want: "error",
		},
		{
			name: "panic",
			run: func(context.Context, *harness.Runtime) (cases.Outcome, error) {
				panic("boom")
			},
			want: "error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123"}, nil)
			cleaned := false
			g := bind.Group{Run: func(ctx context.Context, rt *harness.Runtime, _ cases.Case) (cases.Outcome, error) {
				rt.Cleanup(func(context.Context) error {
					cleaned = true
					return nil
				})
				return tt.run(ctx, rt)
			}}
			res := runOne(context.Background(), rt, g, cases.Case{ID: "S3-CLEANUP", Expect: cases.Expect{HTTP: 200}})
			if !cleaned {
				t.Fatal("cleanup did not run")
			}
			if res.Status != tt.want {
				t.Fatalf("status %s want %s: %s", res.Status, tt.want, res.Reason)
			}
		})
	}
}

func TestRunOneCleanupFailureIsHarnessError(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123"}, nil)
	g := bind.Group{Run: func(_ context.Context, rt *harness.Runtime, _ cases.Case) (cases.Outcome, error) {
		rt.Cleanup(func(context.Context) error { return errors.New("delete failed") })
		return cases.Outcome{HTTP: 200}, nil
	}}
	res := runOne(context.Background(), rt, g, cases.Case{ID: "S3-CLEANUP", Expect: cases.Expect{HTTP: 200}})
	if res.Status != "error" || res.Reason != "cleanup: [delete failed]" {
		t.Fatalf("status %s reason %q", res.Status, res.Reason)
	}
}

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
