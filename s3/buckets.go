package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Buckets() bind.Group {
	return bind.Group{
		Service: "s3",
		Name:    "buckets",
		Run:     runBuckets,
		Cases: []cases.Case{
			{
				ID: "S3-LB-001", Service: "s3", Group: "buckets", Op: "ListBuckets", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("ListBuckets"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-LB-101", Service: "s3", Group: "buckets", Op: "ListBuckets", Kind: cases.KindError, Suite: cases.SuiteRaw,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Impl: "yes",
			},
			{
				ID: "S3-BKT-001", Service: "s3", Group: "buckets", Op: "CreateBucket", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("CreateBucket"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-BKT-002", Service: "s3", Group: "buckets", Op: "HeadBucket", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("HeadBucket"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-BKT-003", Service: "s3", Group: "buckets", Op: "DeleteBucket", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("DeleteBucket"), Expect: cases.Expect{HTTP: 204}, Impl: "yes",
			},
			{
				ID: "S3-BKT-101", Service: "s3", Group: "buckets", Op: "CreateBucket", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidBucketName"}, Impl: "yes",
			},
			{
				ID: "S3-BKT-102", Service: "s3", Group: "buckets", Op: "HeadBucket", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Head: true}, Impl: "yes",
			},
			{
				ID: "S3-BKT-103", Service: "s3", Group: "buckets", Op: "DeleteBucket", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 409, Code: "BucketNotEmpty"}, Impl: "yes",
			},
			{
				ID: "S3-BKT-104", Service: "s3", Group: "buckets", Op: "CreateBucket", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("CreateBucket"), Expect: cases.Expect{HTTP: 409, Code: "BucketAlreadyOwnedByYou"},
				Impl: "yes",
			},
			{
				ID: "S3-BKT-105", Service: "s3", Group: "buckets", Op: "DeleteBucket", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"}, Impl: "yes",
			},
		},
	}
}

func runBuckets(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	switch c.ID {
	case "S3-LB-001":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.ListBuckets(ctx, &s3.ListBucketsInput{})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-LB-101":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rt.Cfg.S3Endpoint+"/", nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("runBuckets", err)
		}
		resp, err := rt.Clients.Raw.Do(req)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("unsigned list: %w", err)
		}
		defer closeChecked(resp.Body)
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return cases.Outcome{}, wrapErr("runBuckets", err)
		}
		return cases.Outcome{
			HTTP: resp.StatusCode, Code: xmlTag(body, "Code"), Message: xmlTag(body, "Message"),
			Fields: map[string]string{},
		}, nil
	case "S3-BKT-001":
		name := rt.Name("b1")
		var created bool
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(name),
				CreateBucketConfiguration: &types.CreateBucketConfiguration{
					LocationConstraint: types.BucketLocationConstraint(rt.Region()),
				},
			})
			created = err == nil
			return wrapErr("runBuckets", err)
		})
		if created && harness.AllowMutation(true, nil, nil, func(string) bool { return false }) {
			rt.Cleanup(func(ctx context.Context) error {
				return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
					_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
					return ignoreNoSuch(err)
				})
			})
			if err := rt.Track("bucket", name, name); err != nil {
				return callOut, wrapErr("runBuckets", err)
			}
		}
		return callOut, wrapErr("runBuckets", callErr)
	case "S3-BKT-002":
		name := rt.Name("b2")
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-BKT-003":
		name := rt.Name("b3")
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-BKT-101":
		const name = "NO"
		var created bool
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
			created = err == nil
			return wrapErr("runBuckets", err)
		})
		if created {
			rt.Cleanup(func(ctx context.Context) error {
				return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
					_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
					return ignoreNoSuch(err)
				})
			})
			if err := rt.Track("bucket", name, name); err != nil {
				return callOut, wrapErr("runBuckets", err)
			}
		}
		return callOut, wrapErr("runBuckets", callErr)
	case "S3-BKT-102":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(rt.Name("missing"))})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-BKT-103":
		name := rt.Name("b4")
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader("x"),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put object: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-BKT-104":
		name := rt.Name("b5")
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	case "S3-BKT-105":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(rt.Name("missing"))})
				return wrapErr("runBuckets", err)
			})
			return callOut, wrapErr("runBuckets", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func xmlTag(body []byte, tag string) string {
	text := string(body)
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	i := strings.Index(text, open)
	if i < 0 {
		return ""
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
