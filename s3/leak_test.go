package s3

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

func TestUnexpectedBucketCreateIsDeleted(t *testing.T) {
	var mu sync.Mutex
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				panic(err)
			}
		}
		if missingBucket(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodDelete && strings.Trim(r.URL.Path, "/") == "NO" {
			mu.Lock()
			deleted = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPut && strings.Trim(r.URL.Path, "/") == "NO" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		S3Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123",
		OutDir: t.TempDir(), AccountID: "123456789012",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	var spec cases.Case
	for _, c := range Buckets().Cases {
		if c.ID == "S3-BKT-101" {
			spec = c
		}
	}
	if _, err := runBuckets(context.Background(), rt, spec); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !deleted {
		t.Fatal("successful create left bucket NO")
	}
}
