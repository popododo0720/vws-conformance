package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestBucketPolicyProbeNamesEffectAndPrincipal(t *testing.T) {
	doc := bucketPolicy()
	for _, part := range []string{`"Effect"`, `"Principal"`, `"Action"`, `"Resource"`} {
		if !strings.Contains(doc, part) {
			t.Fatalf("policy %s lacks %s", doc, part)
		}
	}
}

func TestLifecycleSuccessProbeIncludesAction(t *testing.T) {
	kept := false
	for _, c := range Gaps().Cases {
		if c.ID != "S3-GAP-010" {
			continue
		}
		kept = c.Expect.HTTP == 200 && c.Expect.Code == ""
	}
	if !kept {
		t.Fatal("S3-GAP-010 expectation changed")
	}
	body := lifecycleBody(t, "S3-GAP-010")
	if !hasLifecycleAction(body) {
		t.Fatalf("no action in %s", body)
	}
}

func TestLifecycleMalformedCaseExpectsSchemaError(t *testing.T) {
	var found cases.Case
	ok := false
	for _, c := range Gaps().Cases {
		if c.ID == "S3-GAP-016" {
			found = c
			ok = true
		}
	}
	if !ok {
		t.Fatal("missing malformed lifecycle case")
	}
	if found.Op != "PutBucketLifecycleConfiguration" || found.Expect.HTTP != 400 || found.Expect.Code != "MalformedXML" {
		t.Fatalf("%s %d %s", found.Op, found.Expect.HTTP, found.Expect.Code)
	}
	body := lifecycleBody(t, "S3-GAP-016")
	if hasLifecycleAction(body) {
		t.Fatalf("malformed case included an action: %s", body)
	}
}

func hasLifecycleAction(body string) bool {
	for _, tag := range []string{
		"Expiration", "Transition", "NoncurrentVersionExpiration",
		"NoncurrentVersionTransition", "AbortIncompleteMultipartUpload",
	} {
		if strings.Contains(body, tag) {
			return true
		}
	}
	return false
}

func lifecycleBody(t *testing.T, id string) string {
	t.Helper()
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(strings.ToLower(r.URL.RawQuery), "lifecycle") {
			body = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		S3Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	var chosen cases.Case
	for _, c := range Gaps().Cases {
		if c.ID == id {
			chosen = c
		}
	}
	if chosen.ID == "" {
		t.Fatalf("no case %s", id)
	}
	if _, err := runGaps(context.Background(), rt, chosen); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		t.Fatal("lifecycle request was not sent")
	}
	return body
}
