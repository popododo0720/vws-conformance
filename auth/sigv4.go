package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func SigV4() bind.Group {
	skip := cases.SkipIAM
	a := func(id, op string, exp cases.Expect, docURL, skipReason string) cases.Case {
		return cases.Case{
			ID: id, Service: "auth", Group: "sigv4", Op: op, Kind: cases.KindAuth, Suite: cases.SuiteRaw,
			DocURL: docURL, Expect: exp, Skip: skipReason, Impl: "yes",
		}
	}
	return bind.Group{Service: "auth", Name: "sigv4", Run: run, Cases: []cases.Case{
		a("AUTH-001", "ListBuckets", cases.Expect{HTTP: 200}, cases.DocSigV4, ""),
		a("AUTH-002", "ListBuckets", cases.Expect{HTTP: 403, Code: "SignatureDoesNotMatch"}, cases.DocSigV4, ""),
		a("AUTH-003", "ListBuckets", cases.Expect{HTTP: 403, Code: "RequestTimeTooSkewed"}, cases.DocSigV4, ""),
		a("AUTH-004", "ListBuckets", cases.Expect{HTTP: 400, Code: "AuthorizationHeaderMalformed"}, cases.DocSigV4, ""),
		a("AUTH-005", "ListBuckets", cases.Expect{HTTP: 400, Code: "AuthorizationHeaderMalformed"}, cases.DocS3Errors, ""),
		a("AUTH-006", "ListBuckets", cases.Expect{HTTP: 403, Code: "InvalidAccessKeyId"}, cases.DocSigV4, ""),
		a("AUTH-007", "ListBuckets", cases.Expect{HTTP: 403, Code: "AccessDenied"}, cases.DocSigV4, ""),
		a("AUTH-008", "ListBuckets", cases.Expect{HTTP: 403, Code: "InvalidAccessKeyId"}, cases.DocSigV4, skip),
		a("AUTH-011", "GetCallerIdentity", cases.Expect{HTTP: 200}, cases.DocSigV4, skip),
		a("AUTH-013", "GetCallerIdentity", cases.Expect{HTTP: 403, Code: "ExpiredToken"}, cases.DocSigV4, skip),
	}}
}

func skewedSigningTime(now time.Time) time.Time {
	return now.Add(-16 * time.Minute)
}

func run(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	switch c.ID {
	case "AUTH-001":
		return signed(ctx, rt, "s3", rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret, time.Now(), false)
	case "AUTH-002":
		return signed(ctx, rt, "s3", rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret, time.Now(), true)
	case "AUTH-003":
		return signed(ctx, rt, "s3", rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret, skewedSigningTime(time.Now()), false)
	case "AUTH-004":
		return signed(ctx, rt, "s3", "us-east-1", rt.Cfg.AccessKey, rt.Cfg.Secret, time.Now(), false)
	case "AUTH-005":
		return signed(ctx, rt, "ec2", rt.Cfg.Region, rt.Cfg.AccessKey, rt.Cfg.Secret, time.Now(), false)
	case "AUTH-006":
		return signed(ctx, rt, "s3", rt.Cfg.Region, "AKIAUNKNOWN00000000", rt.Cfg.Secret, time.Now(), false)
	case "AUTH-007":
		if rt.Clients == nil || rt.Clients.Raw == nil {
			return cases.Outcome{}, fmt.Errorf("http client is unset")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rt.Cfg.S3Endpoint+"/", nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("run", err)
		}
		resp, err := rt.Clients.Raw.Do(req)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("unsigned: %w", err)
		}
		defer closeChecked(resp.Body)
		return readErr(resp, rt.Cfg.AccessKey, rt.Cfg.Secret, rt.Cfg.Session)
	default:
		return cases.Outcome{}, fmt.Errorf("auth case %s is not runnable", c.ID)
	}
}

func signed(ctx context.Context, rt *harness.Runtime, service, region, key, secret string, when time.Time,
	flip bool,
) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.Raw == nil {
		return cases.Outcome{}, fmt.Errorf("http client is unset")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rt.Cfg.S3Endpoint+"/", nil)
	if err != nil {
		return cases.Outcome{}, wrapErr("signed", err)
	}
	// A session token is sent as the signed X-Amz-Security-Token header, as the SigV4 guide requires.
	// S3 also requires the payload hash as the signed X-Amz-Content-Sha256 header.
	payloadHash := harness.PayloadHash(nil)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, aws.Credentials{AccessKeyID: key, SecretAccessKey: secret, SessionToken: rt.Cfg.Session},
		req, payloadHash,
		service, region, when)
	if err != nil {
		return cases.Outcome{}, fmt.Errorf("sign: %w", err)
	}
	if flip {
		authz := req.Header.Get("Authorization")
		if i := strings.LastIndex(authz, "Signature="); i >= 0 {
			sig := authz[i+len("Signature="):]
			if len(sig) > 0 {
				replacement := "0"
				if sig[0] == '0' {
					replacement = "1"
				}
				req.Header.Set("Authorization", authz[:i+len("Signature=")]+replacement+sig[1:])
			}
		}
	}
	resp, err := rt.Clients.Raw.Do(req)
	if err != nil {
		return cases.Outcome{}, fmt.Errorf("signed call: %w", err)
	}
	defer closeChecked(resp.Body)
	return readErr(resp, key, secret, rt.Cfg.Session)
}

// readErr parses an XML error body. The body and message are redacted before
// they reach Raw, which the engine writes to the CI raw artifact.
func readErr(resp *http.Response, secrets ...string) (cases.Outcome, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return cases.Outcome{}, wrapErr("readErr", err)
	}
	text := string(body)
	code := ""
	for _, tag := range []string{"Code", "code"} {
		open := "<" + tag + ">"
		end := "</" + tag + ">"
		if i := strings.Index(text, open); i >= 0 {
			rest := text[i+len(open):]
			if j := strings.Index(rest, end); j >= 0 {
				code = rest[:j]
				break
			}
		}
	}
	msg := ""
	if i := strings.Index(text, "<Message>"); i >= 0 {
		rest := text[i+len("<Message>"):]
		if j := strings.Index(rest, "</Message>"); j >= 0 {
			msg = rest[:j]
		}
	}
	return cases.Outcome{
		HTTP: resp.StatusCode, Code: code, Message: harness.Redact(msg, secrets...), Fields: map[string]string{},
		Raw: harness.Redact(text, secrets...),
	}, nil
}

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}

func closeChecked(c io.Closer) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close: %v\n", err)
	}
}
