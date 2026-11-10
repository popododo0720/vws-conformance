package engine

import (
	"path/filepath"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/report"
)

func TestReportSchemaTargetNames(t *testing.T) {
	schema := filepath.Join(repoRoot(t), "schema", "report.schema.json")
	for name, ok := range map[string]bool{
		"vws-test": true, "local-fake": true, "local-pve": true, "pve-test": true, "local-grpc": false,
	} {
		rep := report.New("conf-20261015T120000Z-abc123", report.Target{
			Name: name, Region: "vws-1", ConsoleAPIVersion: "local",
		}, report.Tools{Go: "go1.26.9", SDK: "v1.47.3", CLI: "not-installed"})
		rep.Finish()
		b, err := rep.JSON()
		if err != nil {
			t.Fatal(err)
		}
		if err := report.Validate(schema, b); (err == nil) != ok {
			t.Errorf("target %q: err=%v, want ok=%v", name, err, ok)
		}
	}
}
