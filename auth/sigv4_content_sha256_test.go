package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

// TestSignedContentSHA256 checks that a raw signed request carries the payload
// hash as a signed X-Amz-Content-Sha256 header, as S3 requires for every SigV4
// request. It covers the S3 scope and the wrong-service scope AUTH-005 signs
// with.
func TestSignedContentSHA256(t *testing.T) {
	// SHA-256 of an empty body, which S3 requires in X-Amz-Content-Sha256.
	const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	for _, service := range []string{"s3", "ec2"} {
		t.Run(service, func(t *testing.T) {
			var got http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			rt := harness.NewRuntime(harness.Config{
				AccessKey: "AKIDEXAMPLE", Secret: "test-secret",
				Region: "vws-1", S3Endpoint: srv.URL,
			}, &harness.Clients{Raw: srv.Client()})
			out, err := signed(context.Background(), rt, service, rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret,
				time.Now(), false)
			if err != nil {
				t.Fatalf("signed: %v", err)
			}
			if out.HTTP != http.StatusOK {
				t.Fatalf("status = %d, want 200", out.HTTP)
			}
			if hash := got.Get("X-Amz-Content-Sha256"); hash != emptyPayloadHash {
				t.Fatalf("X-Amz-Content-Sha256 = %q, want %q", hash, emptyPayloadHash)
			}
			authz := got.Get("Authorization")
			signedHeaders := ""
			if i := strings.Index(authz, "SignedHeaders="); i >= 0 {
				signedHeaders, _, _ = strings.Cut(authz[i+len("SignedHeaders="):], ",")
			}
			for _, h := range strings.Split(signedHeaders, ";") {
				if h == "x-amz-content-sha256" {
					return
				}
			}
			t.Fatalf("x-amz-content-sha256 is not in SignedHeaders %q", signedHeaders)
		})
	}
}
