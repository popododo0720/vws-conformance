package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestRepeatedPaginationTokenIsReported(t *testing.T) {
	rt := listingRuntime(t, func(page int) string {
		return listPage("a/x", "again", true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := runListings(ctx, rt, listingCase("S3-LO-002"))
	if err == nil || !strings.Contains(err.Error(), "again") {
		t.Fatalf("got %v", err)
	}
}

func TestDistinctPaginationTokensComplete(t *testing.T) {
	pages := []string{
		listPage("a/x", "t1", true),
		listPage("a/y", "t2", true),
		listPage("b", "", false),
	}
	var n int
	rt := listingRuntime(t, func(page int) string {
		if page >= len(pages) {
			t.Errorf("page %d", page)
			return listPage("b", "", false)
		}
		body := pages[page]
		n = page + 1
		return body
	})
	out, err := runListings(context.Background(), rt, listingCase("S3-LO-002"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Fields["keys"] != "a/x,a/y,b" || n < 2 {
		t.Fatalf("keys %q pages %s calls %d", out.Fields["keys"], out.Fields["pages"], n)
	}
}

func listingCase(id string) cases.Case {
	for _, c := range Listings().Cases {
		if c.ID == id {
			return c
		}
	}
	return cases.Case{ID: id}
}

func listingRuntime(t *testing.T, page func(int) string) *harness.Runtime {
	t.Helper()
	var lists int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingBucket(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "list-type=2") {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, page(lists)); writeErr != nil {
				panic(writeErr)
			}
			lists++
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		S3Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"}, Target: "local-fake",
		RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return harness.NewRuntime(cfg, cl)
}

func listPage(key, token string, truncated bool) string {
	trunc := "false"
	next := ""
	if truncated {
		trunc = "true"
		next = "<NextContinuationToken>" + token + "</NextContinuationToken>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">` +
		`<Name>b</Name><KeyCount>1</KeyCount><MaxKeys>1</MaxKeys><IsTruncated>` + trunc + `</IsTruncated>` +
		next + `<Contents><Key>` + key + `</Key><LastModified>2026-10-15T00:00:00.000Z</LastModified>` +
		`<ETag>&quot;abc&quot;</ETag><Size>1</Size><StorageClass>STANDARD</StorageClass></Contents>` +
		`</ListBucketResult>`
}
