package auth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func SigV4() bind.Group {
	skip := cases.SkipIAM
	a := func(id, op string, exp cases.Expect, skipReason string) cases.Case {
		return cases.Case{
			ID: id, Service: "auth", Group: "sigv4", Op: op, Kind: cases.KindAuth, Suite: cases.SuiteRaw,
			DocURL: cases.DocSigV4, Expect: exp, Skip: skipReason, Impl: "yes",
		}
	}
	return bind.Group{Service: "auth", Name: "sigv4", Run: run, Cases: []cases.Case{
		a("AUTH-001", "ListBuckets", cases.Expect{HTTP: 200}, ""),
		a("AUTH-002", "ListBuckets", cases.Expect{HTTP: 403, Code: "SignatureDoesNotMatch"}, ""),
		a("AUTH-003", "ListBuckets", cases.Expect{HTTP: 403, Code: "RequestTimeTooSkewed"}, ""),
		a("AUTH-004", "ListBuckets", cases.Expect{HTTP: 400, Code: "AuthorizationHeaderMalformed"}, ""),
		a("AUTH-005", "ListBuckets", cases.Expect{HTTP: 403, Code: "SignatureDoesNotMatch"}, ""),
		a("AUTH-006", "ListBuckets", cases.Expect{HTTP: 403, Code: "InvalidAccessKeyId"}, ""),
		a("AUTH-007", "ListBuckets", cases.Expect{HTTP: 403, Code: "AccessDenied"}, ""),
		a("AUTH-008", "ListBuckets", cases.Expect{HTTP: 403, Code: "InvalidAccessKeyId"}, skip),
		a("AUTH-009", "GetObject", cases.Expect{HTTP: 200}, ""),
		a("AUTH-010", "PutObject", cases.Expect{HTTP: 200}, ""),
		a("AUTH-011", "GetCallerIdentity", cases.Expect{HTTP: 200}, skip),
		a("AUTH-012", "AssumeRole", cases.Expect{HTTP: 200}, skip),
		a("AUTH-013", "GetCallerIdentity", cases.Expect{HTTP: 403, Code: "ExpiredToken"}, skip),
		a("AUTH-014", "RunInstances", cases.Expect{HTTP: 403, Code: "UnauthorizedOperation"}, skip),
		a("AUTH-015", "PutObject", cases.Expect{HTTP: 403, Code: "AccessDenied"}, skip),
		a("AUTH-016", "AssumeRole", cases.Expect{HTTP: 403, Code: "AccessDenied"}, skip),
		a("AUTH-017", "AssumeRole", cases.Expect{HTTP: 403, Code: "AccessDenied"}, skip),
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
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rt.Cfg.S3Endpoint+"/", nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("run", err)
		}
		resp, err := rt.Clients.Raw.Do(req)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("unsigned: %w", err)
		}
		defer closeChecked(resp.Body)
		return readErr(resp)
	case "AUTH-009":
		if rt.Clients.S3 == nil {
			return cases.Outcome{}, fmt.Errorf("s3 client is unset")
		}
		name := rt.Name("auth")
		if _, err := rt.Clients.S3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)}); err != nil &&
			!strings.Contains(err.Error(), "BucketAlready") {
			return cases.Outcome{}, fmt.Errorf("bucket: %w", err)
		}
		if _, err := rt.Clients.S3.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytes.NewReader([]byte("v")),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		ps := s3.NewPresignClient(rt.Clients.S3)
		req, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(name), Key: aws.String("k")},
			s3.WithPresignExpires(time.Minute))
		if err != nil {
			return cases.Outcome{}, wrapErr("run", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("run", err)
		}
		resp, err := rt.Clients.Raw.Do(httpReq)
		if err != nil {
			return cases.Outcome{}, wrapErr("run", err)
		}
		defer closeChecked(resp.Body)
		return readErr(resp)
	case "AUTH-010":
		if rt.Clients.S3 == nil {
			return cases.Outcome{}, fmt.Errorf("s3 client is unset")
		}
		name := rt.Name("stream")
		if _, err := rt.Clients.S3.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)}); err != nil &&
			!strings.Contains(err.Error(), "BucketAlready") {
			return cases.Outcome{}, fmt.Errorf("bucket: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := rt.Clients.S3.PutObject(ctx, &s3.PutObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"),
					Body: bytes.NewReader([]byte("stream-body")),
				})
				return wrapErr("run", err)
			})
			return callOut, wrapErr("run", callErr)
		}
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
	signer := v4.NewSigner()
	err = signer.SignHTTP(ctx, aws.Credentials{AccessKeyID: key, SecretAccessKey: secret}, req, harness.PayloadHash(nil),
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
	return readErr(resp)
}

func readErr(resp *http.Response) (cases.Outcome, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return cases.Outcome{}, wrapErr("readErr", err)
	}
	text := string(body)
	code := ""
	for _, tag := range []string{"Code", "code"} {
		open := "<" + tag + ">"
		close := "</" + tag + ">"
		if i := strings.Index(text, open); i >= 0 {
			rest := text[i+len(open):]
			if j := strings.Index(rest, close); j >= 0 {
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
		HTTP: resp.StatusCode, Code: code, Message: msg, Fields: map[string]string{},
		Raw: harness.Redact(text),
	}, nil
}
