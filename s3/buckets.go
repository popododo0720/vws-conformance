package s3

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Buckets() bind.Group {
	return bind.Group{
		Service: "s3", Name: "buckets", Run: runBuckets,
		Cases: []cases.Case{
			{
				ID: "S3-LB-001", Service: "s3", Group: "buckets", Op: "ListBuckets", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocS3("ListBuckets"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-LB-101", Service: "s3", Group: "buckets", Op: "ListBuckets", Kind: cases.KindError,
				Suite: cases.SuiteRaw, DocURL: cases.DocS3Errors,
				Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Impl: "yes",
			},
			{
				ID: "S3-BKT-102", Service: "s3", Group: "buckets", Op: "HeadBucket", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Head: true}, Impl: "yes",
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
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListBuckets(ctx, &s3.ListBucketsInput{})
			return wrapErr("runBuckets", err)
		})
		return o, wrapErr("runBuckets", err)
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
	case "S3-BKT-102":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(rt.Name("missing"))})
			return wrapErr("runBuckets", err)
		})
		return o, wrapErr("runBuckets", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
