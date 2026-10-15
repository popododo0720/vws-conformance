package s3

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Listings() bind.Group {
	return bind.Group{
		Service: "s3", Name: "listings", Run: runListings,
		Cases: []cases.Case{
			{ID: "S3-LO-001", Service: "s3", Group: "listings", Op: "ListObjects", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocS3("ListObjects"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"keys": "a/x,a/y"}}, Impl: "yes"},
			{ID: "S3-LO-002", Service: "s3", Group: "listings", Op: "ListObjectsV2", Kind: cases.KindPagination, Suite: cases.SuiteSDK, DocURL: cases.DocS3("ListObjectsV2"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"keys": "^a/x,a/y,b$", "pages": "^[2-9]"}}, Impl: "yes"},
			{ID: "S3-LO-003", Service: "s3", Group: "listings", Op: "ListObjectsV2", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocS3("ListObjectsV2"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"prefix": "a/"}}, Impl: "yes"},
			{ID: "S3-LO-101", Service: "s3", Group: "listings", Op: "ListObjectsV2", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"}, Impl: "yes"},
			{ID: "S3-LO-102", Service: "s3", Group: "listings", Op: "ListObjects", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"}, Impl: "yes"},
		},
	}
}

func runListings(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	name := rt.Name("list")
	if c.ID != "S3-LO-101" && c.ID != "S3-LO-102" {
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
		for _, key := range []string{"a/x", "a/y", "b"} {
			if _, err := api.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(name), Key: aws.String(key), Body: bytesReader("v")}); err != nil {
				return cases.Outcome{}, fmt.Errorf("put %s: %w", key, err)
			}
		}
	}
	switch c.ID {
	case "S3-LO-001":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListObjects(ctx, &s3.ListObjectsInput{Bucket: aws.String(name), Delimiter: aws.String("/")})
			return err
		})
		if err != nil {
			return o, err
		}
		out, err := api.ListObjects(ctx, &s3.ListObjectsInput{Bucket: aws.String(name), Prefix: aws.String("a/")})
		if err != nil {
			return o, fmt.Errorf("list check: %w", err)
		}
		o.Fields["keys"] = joinKeys(out.Contents)
		return o, nil
	case "S3-LO-002":
		p := s3.NewListObjectsV2Paginator(api, &s3.ListObjectsV2Input{Bucket: aws.String(name), MaxKeys: aws.Int32(1)})
		var keys []string
		pages := 0
		var last int
		for p.HasMorePages() {
			page, err := p.NextPage(ctx)
			if err != nil {
				return cases.Outcome{}, fmt.Errorf("page: %w", err)
			}
			pages++
			last = 200
			for _, obj := range page.Contents {
				keys = append(keys, aws.ToString(obj.Key))
			}
		}
		sort.Strings(keys)
		return cases.Outcome{HTTP: last, Fields: map[string]string{"keys": join(keys), "pages": strconv.Itoa(pages)}}, nil
	case "S3-LO-003":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
				Bucket: aws.String(name), Delimiter: aws.String("/"), EncodingType: types.EncodingTypeUrl,
			})
			if err == nil {
				_ = out
			}
			return err
		})
		if err != nil {
			return o, err
		}
		out, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(name), Delimiter: aws.String("/")})
		if err != nil {
			return o, err
		}
		if len(out.CommonPrefixes) == 0 {
			o.Fields["prefix"] = ""
		} else {
			o.Fields["prefix"] = aws.ToString(out.CommonPrefixes[0].Prefix)
		}
		return o, nil
	case "S3-LO-101":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(rt.Name("missing"))})
			return err
		})
	case "S3-LO-102":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListObjects(ctx, &s3.ListObjectsInput{Bucket: aws.String(rt.Name("missing"))})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func joinKeys(objs []types.Object) string {
	var keys []string
	for _, o := range objs {
		keys = append(keys, aws.ToString(o.Key))
	}
	return join(keys)
}

func join(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		out += k
	}
	return out
}
