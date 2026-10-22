package s3

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Listings() bind.Group {
	return bind.Group{
		Service: "s3", Name: "listings", Run: runListings,
		Cases: []cases.Case{
			{
				ID: "S3-LO-101", Service: "s3", Group: "listings", Op: "ListObjectsV2", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"},
				Impl: "yes",
			},
			{
				ID: "S3-LO-102", Service: "s3", Group: "listings", Op: "ListObjects", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"},
				Impl: "yes",
			},
		},
	}
}

func runListings(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	name := bucketName(rt.Name(c.ID))
	switch c.ID {
	case "S3-LO-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(name)})
			return wrapErr("runListings", err)
		})
		return o, wrapErr("runListings", err)
	case "S3-LO-102":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListObjects(ctx, &s3.ListObjectsInput{Bucket: aws.String(name)})
			return wrapErr("runListings", err)
		})
		return o, wrapErr("runListings", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
