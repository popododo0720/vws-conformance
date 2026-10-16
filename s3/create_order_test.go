package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestCreateFailureDoesNotScheduleDeletion(t *testing.T) {
	const runID = "conf-20261016T120000Z-aabbcc"
	rows := []struct {
		name string
		run  func(t *testing.T, rt *harness.Runtime)
	}{
		{name: "S3-BKT-001", run: func(t *testing.T, rt *harness.Runtime) {
			t.Helper()
			got, err := runBuckets(context.Background(), rt, cases.Case{ID: "S3-BKT-001"})
			if err != nil || got.HTTP != http.StatusConflict || got.Code != "BucketAlreadyOwnedByYou" {
				t.Fatalf("outcome http %d code %q err %v", got.HTTP, got.Code, err)
			}
		}},
		{name: "createFresh", run: func(t *testing.T, rt *harness.Runtime) {
			t.Helper()
			err := createFresh(context.Background(), rt, rt.Clients.S3, rt.Name("fresh"))
			if err == nil {
				t.Fatal("createFresh returned nil")
			}
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var mu sync.Mutex
			deleted := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				if r.Method == http.MethodDelete && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
					mu.Lock()
					deleted = true
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method == http.MethodPut && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
					w.WriteHeader(http.StatusConflict)
					name := strings.Trim(r.URL.Path, "/")
					if _, err := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error>`+
						`<Code>BucketAlreadyOwnedByYou</Code>`+
						`<Message>Your previous request to create the named bucket succeeded and you already own it.</Message>`+
						`<BucketName>`+name+`</BucketName><RequestId>r</RequestId></Error>`); err != nil {
						panic(err)
					}
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			cfg := harness.Config{
				AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
				S3Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
				AllowedAccounts: []string{"123456789012"},
				Target:          "local-fake", RunID: runID, OutDir: t.TempDir(), AccountID: "123456789012",
			}
			cl, err := harness.Load(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			rt := harness.NewRuntime(cfg, cl)
			row.run(t, rt)
			if err := rt.Clean.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			gotDeleted := deleted
			mu.Unlock()
			ledger, readErr := os.ReadFile(filepath.Join(rt.Cfg.OutDir, "ledger.jsonl"))
			if readErr != nil && !os.IsNotExist(readErr) {
				t.Fatal(readErr)
			}
			ledgerText := string(ledger)
			named := strings.Contains(ledgerText, rt.Name("b1")) || strings.Contains(ledgerText, rt.Name("fresh"))
			if gotDeleted || named {
				t.Fatalf("deleted %v ledger %s", gotDeleted, ledgerText)
			}
		})
	}
}
