package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestAuthBucketIsDeletedByItsCreator(t *testing.T) {
	const runID = "conf-20261016T120000Z-aabbcc"
	var mu sync.Mutex
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
			mu.Lock()
			deleted[strings.Trim(r.URL.Path, "/")] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><ok/>`); writeErr != nil {
			panic(writeErr)
		}
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
	for _, id := range []string{"AUTH-009", "AUTH-010"} {
		if _, callErr := run(context.Background(), rt, cases.Case{ID: id}); callErr != nil {
			t.Fatalf("%s %v", id, callErr)
		}
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{rt.Name("auth"), rt.Name("stream")} {
		if !deleted[name] {
			t.Fatalf("creator left %s deleted %v", name, deleted)
		}
	}
}
