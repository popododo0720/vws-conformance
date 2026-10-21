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
			signed := map[string]bool{}
			for _, h := range strings.Split(signedHeaders, ";") {
				signed[h] = true
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
// scope: S3 treats it as a malformed Authorization header.
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
}

// TestSignatureMismatchRawHidesCredentials checks that a SignatureDoesNotMatch
// body echoing the access key ID and session token never reaches Raw, which the
// engine writes verbatim to the raw/<case>.txt CI artifact.
func TestSignatureMismatchRawHidesCredentials(t *testing.T) {
	const (
		key    = "AKIDEXAMPLECONFTEST01"
		secret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		token  = "test-session-token-123456"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		if _, err := w.Write([]byte("<Error><Code>SignatureDoesNotMatch</Code>" +
			"<Message>signature mismatch for " + key + "</Message>" +
			"<AWSAccessKeyId>" + key + "</AWSAccessKeyId>" +
			"<CanonicalRequest>GET\n/\n\nx-amz-security-token:" + r.Header.Get("X-Amz-Security-Token") +
			"\n</CanonicalRequest>" +
			"<SignatureProvided>" + r.Header.Get("Authorization") + "</SignatureProvided></Error>")); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{
		AccessKey: key, Secret: secret, Session: token, Region: "vws-1", S3Endpoint: srv.URL,
	}, &harness.Clients{Raw: srv.Client()})
	out, err := signed(context.Background(), rt, "s3", rt.Cfg.Region, key, secret, time.Now(), true)
	if err != nil {
		t.Fatalf("signed: %v", err)
	}
	if out.Code != "SignatureDoesNotMatch" {
		t.Fatalf("code = %q, want SignatureDoesNotMatch", out.Code)
	}
	for _, s := range []string{key, secret, token} {
		if strings.Contains(out.Raw, s) || strings.Contains(out.Message, s) {
			t.Fatalf("leaked %q: raw=%s message=%s", s, out.Raw, out.Message)
		}
	}
}
