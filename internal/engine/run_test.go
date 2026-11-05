package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/report"
)

func TestLoadConfigRejectsForeignRunID(t *testing.T) {
	t.Setenv("VWS_CONF_RUN_ID", "prod-bucket")
	_, err := loadConfig()
	if err == nil {
		t.Fatal("accepted prod-bucket")
	}
}

func TestLoadConfigReusesRunID(t *testing.T) {
	t.Setenv("VWS_CONF_RUN_ID", "")
	first, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first.RunID == "" || first.RunID != second.RunID {
		t.Fatalf("%s vs %s", first.RunID, second.RunID)
	}
}

func TestLoadConfigKeepsExplicitRunID(t *testing.T) {
	const id = "conf-20261015T120000Z-abc123"
	t.Setenv("VWS_CONF_RUN_ID", id)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RunID != id {
		t.Fatal(cfg.RunID)
	}
}

var trackedFail = report.Result{
	ID: "EC2-TRACKED", Status: "fail",
	Issue: "https://github.com/popododo0720/vws-conformance/issues/99",
}

func TestParityGate(t *testing.T) {
	tests := []struct {
		name    string
		results []report.Result
		want    string
	}{
		{"untracked", []report.Result{trackedFail, {ID: "EC2-UNTRACKED", Status: "fail"}}, "EC2-UNTRACKED"},
		{"tracked", []report.Result{trackedFail}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rep := report.New("conf-20261015T120000Z-abc123", report.Target{}, report.Tools{})
			for _, res := range tt.results {
				rep.Add(res)
			}
			rep.Finish()

			var logs []string
			untracked := parityGate(rep, func(msg string) { logs = append(logs, msg) })
			if tt.want == "" && len(untracked) != 0 {
				t.Fatalf("untracked = %v, want none", untracked)
			}
			if tt.want != "" && (len(untracked) != 1 || untracked[0].ID != tt.want) {
				t.Fatalf("untracked = %v, want %s", untracked, tt.want)
			}
			if len(logs) != 1 || !strings.Contains(logs[0], "EC2-TRACKED") {
				t.Fatalf("logs = %q, want the tracked case reported as known", logs)
			}
		})
	}
}

func TestRunOneRefusesCasesAfterShutdown(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir()}, nil)
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ran := false
	g := bind.Group{Run: func(context.Context, *harness.Runtime, cases.Case) (cases.Outcome, error) {
		ran = true
		return cases.Outcome{}, nil
	}}
	res := runOne(context.Background(), rt, g, cases.Case{ID: "EC2-LATE"})
	if ran || res.Status != "error" || !strings.Contains(res.Reason, "shutting down") {
		t.Fatalf("ran %v result %+v", ran, res)
	}
}

func TestRunOneRunsTheCaseOnItsOwnScope(t *testing.T) {
	rt := harness.NewRuntime(harness.Config{RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir()}, nil)
	var view *harness.Runtime
	g := bind.Group{Run: func(_ context.Context, crt *harness.Runtime, _ cases.Case) (cases.Outcome, error) {
		view = crt
		panic("boom")
	}}
	res := runOne(context.Background(), rt, g, cases.Case{ID: "EC2-PANIC"})
	if res.Status != "error" || res.Reason != "boom" {
		t.Fatalf("%+v", res)
	}
	if view == nil || view == rt {
		t.Fatal("the case ran on the shared runtime, not its own scope")
	}
}
