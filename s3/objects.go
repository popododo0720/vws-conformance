package s3

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Objects() bind.Group {
	return bind.Group{
		Service: "s3", Name: "objects", Run: runObjects,
		Cases: []cases.Case{
			{
				ID: "S3-OBJ-001", Service: "s3", Group: "objects", Op: "PutObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("PutObject"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-002", Service: "s3", Group: "objects", Op: "GetObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("GetObject"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"color": "^blue$"}},
				Impl: "yes",
			},
			{
				ID: "S3-OBJ-003", Service: "s3", Group: "objects", Op: "HeadObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("HeadObject"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-004", Service: "s3", Group: "objects", Op: "DeleteObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("DeleteObject"), Expect: cases.Expect{HTTP: 204}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-005", Service: "s3", Group: "objects", Op: "GetObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("GetObject"), Expect: cases.Expect{HTTP: 206}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-101", Service: "s3", Group: "objects", Op: "GetObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchKey"}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-102", Service: "s3", Group: "objects", Op: "PutObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 400, Code: "BadDigest"}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-103", Service: "s3", Group: "objects", Op: "GetObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 416, Code: "InvalidRange"}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-104", Service: "s3", Group: "objects", Op: "GetObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("GetObject"), Expect: cases.Expect{HTTP: 412, Code: "PreconditionFailed"}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-105", Service: "s3", Group: "objects", Op: "DeleteObject", Kind: cases.KindHappy, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3("DeleteObject"), Expect: cases.Expect{HTTP: 204}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-106", Service: "s3", Group: "objects", Op: "HeadObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Head: true}, Impl: "yes",
			},
			{
				ID: "S3-OBJ-107", Service: "s3", Group: "objects", Op: "DeleteObject", Kind: cases.KindError, Suite: cases.SuiteSDK,
				DocURL: cases.DocS3Errors, Expect: cases.Expect{HTTP: 404, Code: "NoSuchBucket"}, Impl: "yes",
			},
		},
	}
}

func runObjects(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	name := bucketName(rt, c.ID)
	switch c.ID {
	case "S3-OBJ-001", "S3-OBJ-002", "S3-OBJ-003", "S3-OBJ-004", "S3-OBJ-005",
		"S3-OBJ-101", "S3-OBJ-102", "S3-OBJ-103", "S3-OBJ-104", "S3-OBJ-105", "S3-OBJ-106":
		if err := createFresh(ctx, rt, api, name); err != nil {
			return cases.Outcome{}, err
		}
	}
	body := []byte("hello-vws")
	sum := md5.Sum(body)
	good := base64.StdEncoding.EncodeToString(sum[:])
	switch c.ID {
	case "S3-OBJ-001":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutObject(ctx, &s3.PutObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"), Body: bytesReader(string(body)),
					ContentMD5: aws.String(good), Metadata: map[string]string{"color": "blue"},
				})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-002":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)), Metadata: map[string]string{"color": "blue"},
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		var color string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(name), Key: aws.String("k")})
			if err == nil && out != nil {
				color = out.Metadata["color"]
			}
			return wrapErr("runObjects", err)
		})
		if o.Fields == nil {
			o.Fields = map[string]string{}
		}
		o.Fields["color"] = color
		return o, wrapErr("runObjects", err)
	case "S3-OBJ-003":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(name), Key: aws.String("k")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-004":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: aws.String("k")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-005":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetObject(ctx, &s3.GetObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"),
					Range: aws.String("bytes=0-3"),
				})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-101":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(name), Key: aws.String("missing")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-102":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				badMD5 := func() string {
					s := md5.Sum([]byte("not-the-body"))
					return base64.StdEncoding.EncodeToString(s[:])
				}()
				_, err := api.PutObject(ctx, &s3.PutObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"), Body: bytesReader(string(body)),
					ContentMD5: aws.String(badMD5),
				})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-103":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetObject(ctx, &s3.GetObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"),
					Range: aws.String("bytes=100-200"),
				})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-104":
		if _, err := api.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(name), Key: aws.String("k"),
			Body: bytesReader(string(body)),
		}); err != nil {
			return cases.Outcome{}, fmt.Errorf("put: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetObject(ctx, &s3.GetObjectInput{
					Bucket: aws.String(name), Key: aws.String("k"),
					IfMatch: aws.String("\"deadbeef\""),
				})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-105":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: aws.String("never-stored")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-106":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(name), Key: aws.String("missing")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	case "S3-OBJ-107":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: aws.String("k")})
				return wrapErr("runObjects", err)
			})
			return callOut, wrapErr("runObjects", callErr)
		}
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
