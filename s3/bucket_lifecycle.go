package s3

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

const bucketOwnedPattern = `^conf-[0-9]{8}t[0-9]{6}z-[0-9a-f]{6}-bkt-104$`

// BucketLifecycle covers CreateBucket conflicts and a bucket recreated after
// DeleteBucket. These are the only mutating SDK calls outside the cleanup gate
// (popododo0720/vws-conformance#5). Each case uses a run-scoped name and
// registers its own cleanup, so it deletes only a bucket this run created.
func BucketLifecycle() bind.Group {
	return bind.Group{
		Service: "s3", Name: "bucketlifecycle", Run: runBucketLifecycle,
		Cases: []cases.Case{
			{
				ID: "S3-BKT-104", Service: "s3", Group: "bucketlifecycle", Op: "CreateBucket",
				Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocS3("CreateBucket"),
				Expect: cases.Expect{
					HTTP: 409, Code: "BucketAlreadyOwnedByYou",
					Patterns: map[string]string{"BucketName": bucketOwnedPattern},
				},
				Impl: "yes",
			},
			{
				ID: "S3-BKT-106", Service: "s3", Group: "bucketlifecycle", Op: "HeadBucket",
				Kind: cases.KindIdempotency, Suite: cases.SuiteSDK, DocURL: cases.DocS3("HeadBucket"),
				Expect: cases.Expect{HTTP: 200, Head: true}, Impl: "yes",
			},
			{
				ID: "S3-BKT-107", Service: "s3", Group: "bucketlifecycle", Op: "HeadBucket",
				Kind: cases.KindIdempotency, Suite: cases.SuiteSDK, DocURL: cases.DocS3("HeadBucket"),
				Expect: cases.Expect{HTTP: 404, Head: true}, Impl: "yes",
			},
		},
	}
}

func runBucketLifecycle(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	switch c.ID {
	case "S3-BKT-104":
		return runBucketOwnedConflict(ctx, rt, api)
	case "S3-BKT-106":
		return runBucketRecreate(ctx, rt, api, "bkt-106", []bucketStep{
			{"create", 200, true},
			{"delete", 204, false},
			{"create", 200, true},
		})
	case "S3-BKT-107":
		return runBucketRecreate(ctx, rt, api, "bkt-107", []bucketStep{
			{"create", 200, true},
			{"delete", 204, false},
			{"create", 200, true},
			{"delete", 204, false},
			{"create", 200, true},
			{"delete", 204, false},
		})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func runBucketOwnedConflict(ctx context.Context, rt *harness.Runtime, api *s3.Client) (cases.Outcome, error) {
	name := rt.Name("bkt-104")
	created := false
	rt.Cleanup(func(ctx context.Context) error {
		if !created {
			return nil
		}
		return removeBucket(ctx, api, name)
	})
	first, err := createBucket(ctx, rt, api, name)
	if err != nil {
		return first, err
	}
	switch {
	case first.HTTP == 200:
		created = true
	case first.HTTP == 409 && first.Code == "BucketAlreadyOwnedByYou":
		// A reused run ID can leave the bucket from an earlier attempt. It is
		// still this run's name, so cleanup may delete it.
		created = true
	default:
		return first, fmt.Errorf("first create: http %d code %q", first.HTTP, first.Code)
	}
	second, err := createBucket(ctx, rt, api, name)
	if err != nil {
		return second, err
	}
	if second.Fields == nil {
		second.Fields = map[string]string{}
	}
	second.Fields["BucketName"] = xmlTag([]byte(second.Raw), "BucketName")
	return second, nil
}

type bucketStep struct {
	op     string
	want   int
	create bool
}

func runBucketRecreate(ctx context.Context, rt *harness.Runtime, api *s3.Client, suffix string,
	steps []bucketStep,
) (cases.Outcome, error) {
	name := rt.Name(suffix)
	exists := false
	rt.Cleanup(func(ctx context.Context) error {
		if !exists {
			return nil
		}
		return removeBucket(ctx, api, name)
	})
	for _, step := range steps {
		var o cases.Outcome
		var err error
		if step.create {
			o, err = createBucket(ctx, rt, api, name)
		} else {
			o, err = deleteBucket(ctx, rt, api, name)
		}
		if err != nil {
			return o, fmt.Errorf("%s: %w", step.op, err)
		}
		if o.HTTP != step.want {
			return o, fmt.Errorf("%s: http %d want %d, code %q", step.op, o.HTTP, step.want, o.Code)
		}
		exists = step.create
	}
	return headBucket(ctx, rt, api, name)
}

func createBucket(ctx context.Context, rt *harness.Runtime, api *s3.Client, name string) (cases.Outcome, error) {
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(name),
			CreateBucketConfiguration: &types.CreateBucketConfiguration{
				LocationConstraint: types.BucketLocationConstraint(rt.Region()),
			},
		})
		return wrapErr("createBucket", err)
	})
	return o, wrapErr("createBucket", err)
}

func deleteBucket(ctx context.Context, rt *harness.Runtime, api *s3.Client, name string) (cases.Outcome, error) {
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
		return wrapErr("deleteBucket", err)
	})
	return o, wrapErr("deleteBucket", err)
}

func headBucket(ctx context.Context, rt *harness.Runtime, api *s3.Client, name string) (cases.Outcome, error) {
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)})
		return wrapErr("headBucket", err)
	})
	return o, wrapErr("headBucket", err)
}

func removeBucket(ctx context.Context, api *s3.Client, name string) error {
	_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	if err == nil {
		return nil
	}
	var ae smithy.APIError
	if errors.As(err, &ae) && ae.ErrorCode() == "NoSuchBucket" {
		return nil
	}
	return fmt.Errorf("remove bucket %s: %w", name, err)
}
