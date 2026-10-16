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

func TestExistingBucketIsNotDeleted(t *testing.T) {
	var mu sync.Mutex
	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				panic(err)
			}
		}
		w.Header().Set("Content-Type", "application/xml")
		name := strings.Trim(r.URL.Path, "/")
		if r.Method == http.MethodDelete && name == "NO" {
			mu.Lock()
			deleted = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPut {
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
	if _, err := runBuckets(context.Background(), rt, spec); err != nil && !strings.Contains(err.Error(), "preserve") {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil && !strings.Contains(err.Error(), "preserve") {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if deleted {
		t.Fatal("CreateBucket success deleted a bucket that already existed")
	}
}

func TestDuplicateCreateSendsLocationConstraint(t *testing.T) {
	var mu sync.Mutex
	var creates []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodPut && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
			mu.Lock()
			creates = append(creates, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			if _, err := io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code></Error>`); err != nil {
				panic(err)
			}
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
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
		if c.ID == "S3-BKT-104" {
			spec = c
		}
	}
	if _, err := runBuckets(context.Background(), rt, spec); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(creates) < 2 {
		t.Fatalf("creates %d", len(creates))
	}
	second := creates[len(creates)-1]
	if !strings.Contains(second, "LocationConstraint") || !strings.Contains(second, "vws-1") {
		t.Fatalf("second create %s", second)
	}
	if strings.Contains(second, "us-east-1") {
		t.Fatalf("second create uses us-east-1: %s", second)
	}
}

func TestFirstCreateSendsLocationConstraint(t *testing.T) {
	var mu sync.Mutex
	var creates []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == http.MethodPut && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
			mu.Lock()
			creates = append(creates, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			if _, err := io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code></Error>`); err != nil {
				panic(err)
			}
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
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
	if err := createFresh(context.Background(), rt, cl.S3, rt.Name("loc")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(creates) == 0 || !strings.Contains(creates[0], "LocationConstraint") || !strings.Contains(creates[0], "vws-1") {
		t.Fatalf("first create %q", creates)
	}
}
