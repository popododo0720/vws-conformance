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

// TestSignedSessionToken checks that raw signed requests carry the session
// token as a signed X-Amz-Security-Token header when one is configured.
func TestSignedSessionToken(t *testing.T) {
	const token = "test-session-token"
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
			hasSigned := false
			for _, h := range strings.Split(signedHeaders, ";") {
				if h == "x-amz-security-token" {
					hasSigned = true
				}
			}
			if tc.session == "" {
				if got.Get("X-Amz-Security-Token") != "" || hasSigned {
					t.Fatal("X-Amz-Security-Token is set without a session token")
				}
				return
			}
			if got.Get("X-Amz-Security-Token") != tc.session {
				t.Fatal("X-Amz-Security-Token does not carry the configured session token")
			}
			if !hasSigned {
				t.Fatalf("x-amz-security-token is not in SignedHeaders %q", signedHeaders)
			}
		})
	}
}
