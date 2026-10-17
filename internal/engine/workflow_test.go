package engine

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestCIUploadsReportsAfterFailure(t *testing.T) {
	body := workflow(t)
	if !strings.Contains(body, "actions/upload-artifact@v4") {
		t.Fatal("workflow does not upload artifacts")
	}
	if !strings.Contains(body, "if: always()") {
		t.Fatal("artifact upload is skipped when the job fails")
	}
	if !strings.Contains(body, "out/") {
		t.Fatal("artifact upload does not keep out/")
	}
}

func TestCIRunsCommandPackageTests(t *testing.T) {
	body := workflow(t)
	if !strings.Contains(body, "./cmd/...") {
		t.Fatal("unit tests skip ./cmd/...")
	}
}

func workflow(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".github", "workflows", "conformance.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
