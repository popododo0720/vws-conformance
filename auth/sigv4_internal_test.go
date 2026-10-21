package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// TestSignedSessionToken checks that a raw signed request carries the session
// token as a signed X-Amz-Security-Token header when one is configured, and the
// S3 payload hash as a signed X-Amz-Content-Sha256 header.
func TestSignedSessionToken(t *testing.T) {
	const token = "test-session-token"
	// SHA-256 of an empty body, which S3 requires in X-Amz-Content-Sha256.
	const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	for _, tc := range []struct {
		name    string
		session string
	}{
		{name: "with session token", session: token},
		{name: "without session token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			rt := harness.NewRuntime(harness.Config{
				AccessKey: "AKIDEXAMPLE", Secret: "test-secret", Session: tc.session,
				Region: "vws-1", S3Endpoint: srv.URL,
			}, &harness.Clients{Raw: srv.Client()})
			out, err := signed(context.Background(), rt, "s3", rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret,
				time.Now(), false)
			if err != nil {
				t.Fatalf("signed: %v", err)
			}
			if out.HTTP != http.StatusOK {
				t.Fatalf("status = %d, want 200", out.HTTP)
			}
			authz := got.Get("Authorization")
			signedHeaders := ""
			if i := strings.Index(authz, "SignedHeaders="); i >= 0 {
				signedHeaders, _, _ = strings.Cut(authz[i+len("SignedHeaders="):], ",")
			}
			signed := map[string]bool{}
			for _, h := range strings.Split(signedHeaders, ";") {
				signed[h] = true
			}
			if hash := got.Get("X-Amz-Content-Sha256"); hash != emptyPayloadHash {
				t.Fatalf("X-Amz-Content-Sha256 = %q, want %q", hash, emptyPayloadHash)
			}
			if !signed["x-amz-content-sha256"] {
				t.Fatalf("x-amz-content-sha256 is not in SignedHeaders %q", signedHeaders)
			}
			if tc.session == "" {
				if got.Get("X-Amz-Security-Token") != "" || signed["x-amz-security-token"] {
					t.Fatal("X-Amz-Security-Token is set without a session token")
				}
				return
			}
			if got.Get("X-Amz-Security-Token") != tc.session {
				t.Fatal("X-Amz-Security-Token does not carry the configured session token")
			}
			if !signed["x-amz-security-token"] {
				t.Fatalf("x-amz-security-token is not in SignedHeaders %q", signedHeaders)
			}
		})
	}
}

// TestAuth005Expectation pins the contract for a wrong-service credential
// scope: S3 treats it as a malformed Authorization header. It also runs the
// case once, so the fault stays a wrong service scope and cannot drift into
// AUTH-001.
func TestAuth005Expectation(t *testing.T) {
	var got cases.Case
	found := false
	for _, c := range SigV4().Cases {
		if c.ID == "AUTH-005" {
			got, found = c, true
			break
		}
	}
	if !found {
		t.Fatal("AUTH-005 is missing from the cases table")
	}
	if got.Expect.HTTP != http.StatusBadRequest {
		t.Fatalf("HTTP = %d, want %d", got.Expect.HTTP, http.StatusBadRequest)
	}
	if got.Expect.Code != "AuthorizationHeaderMalformed" {
		t.Fatalf("Code = %q, want %q", got.Expect.Code, "AuthorizationHeaderMalformed")
	}
	if got.DocURL != cases.DocS3Errors {
		t.Fatalf("DocURL = %q, want %q", got.DocURL, cases.DocS3Errors)
	}

	var authz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authz = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "test-secret",
		Region: "vws-1", S3Endpoint: srv.URL,
	}, &harness.Clients{Raw: srv.Client()})
	if _, err := run(context.Background(), rt, got); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(authz, "/ec2/aws4_request") {
		t.Fatalf("credential scope = %q, want the ec2 service", authz)
	}
}
