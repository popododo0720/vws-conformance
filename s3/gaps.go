package s3

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Gaps() bind.Group {
	return bind.Group{
		Service: "s3", Name: "gaps", Run: runGaps,
		Cases: []cases.Case{
			gap("S3-GAP-001", "CopyObject", 200, ""),
			gap("S3-GAP-002", "DeleteObjects", 200, ""),
			gap("S3-GAP-003", "PutObjectTagging", 200, ""),
			gap("S3-GAP-004", "GetObjectAcl", 200, ""),
			gap("S3-GAP-005", "PutBucketPolicy", 204, ""),
			gap("S3-GAP-006", "CreateMultipartUpload", 200, ""),
			gap("S3-GAP-007", "PutBucketVersioning", 200, ""),
			gap("S3-GAP-008", "GetBucketLocation", 200, ""),
			gap("S3-GAP-009", "PutBucketCors", 200, ""),
			gap("S3-GAP-010", "PutBucketLifecycleConfiguration", 200, ""),
			{
				ID: "S3-GAP-016", Service: "s3", Group: "gaps", Op: "PutBucketLifecycleConfiguration",
				Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocS3("PutBucketLifecycleConfiguration"),
				Expect: cases.Expect{HTTP: 400, Code: "MalformedXML"}, Impl: "gap",
			},
			gap("S3-GAP-011", "PutBucketWebsite", 200, ""),
			gap("S3-GAP-012", "ListObjectVersions", 200, ""),
			gap("S3-GAP-014", "GetObject", 200, ""),
			gap("S3-GAP-015", "HeadBucket", 200, ""),
		},
	}
}

func gap(id, op string, http int, code string) cases.Case {
	return cases.Case{
		ID: id, Service: "s3", Group: "gaps", Op: op, Kind: cases.KindGap, Suite: cases.SuiteSDK,
		DocURL: cases.DocS3(op), Expect: cases.Expect{HTTP: http, Code: code}, Impl: "gap",
	}
}

func runGaps(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return cases.Outcome{}, fmt.Errorf("s3 client is unset")
	}
	api := rt.Clients.S3
	name := bucketName(rt, c.ID)
	if err := createFresh(ctx, rt, api, name); err != nil {
		return cases.Outcome{}, err
	}
	if _, err := api.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(name), Key: aws.String("k"),
		Body: bytesReader("v"),
	}); err != nil {
		return cases.Outcome{}, fmt.Errorf("put: %w", err)
	}
	switch c.ID {
	case "S3-GAP-001":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CopyObject(ctx, &s3.CopyObjectInput{
					Bucket: aws.String(name), Key: aws.String("c"),
					CopySource: aws.String(name + "/k"),
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-002":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DeleteObjects(ctx, &s3.DeleteObjectsInput{
					Bucket: aws.String(name),
					Delete: &types.Delete{Objects: []types.ObjectIdentifier{{Key: aws.String("k")}}},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-003":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{
					Bucket: aws.String(name), Key: aws.String("k"),
					Tagging: &types.Tagging{TagSet: []types.Tag{{Key: aws.String("a"), Value: aws.String("b")}}},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-004":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetObjectAcl(ctx, &s3.GetObjectAclInput{Bucket: aws.String(name), Key: aws.String("k")})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-005":
		doc := bucketPolicy(name)
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(name), Policy: aws.String(doc)})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-006":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
					Bucket: aws.String(name),
					Key:    aws.String("m"),
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-007":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
					Bucket:                  aws.String(name),
					VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusSuspended},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-008":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.GetBucketLocation(ctx, &s3.GetBucketLocationInput{Bucket: aws.String(name)})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-009":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketCors(ctx, &s3.PutBucketCorsInput{
					Bucket: aws.String(name),
					CORSConfiguration: &types.CORSConfiguration{CORSRules: []types.CORSRule{{
						AllowedMethods: []string{"GET"},
						AllowedOrigins: []string{"*"},
					}}},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-010":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
					Bucket: aws.String(name),
					LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: []types.LifecycleRule{{
						ID: aws.String("expire"), Status: types.ExpirationStatusEnabled,
						Filter:     &types.LifecycleRuleFilter{Prefix: aws.String("")},
						Expiration: &types.LifecycleExpiration{Days: aws.Int32(1)},
					}}},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-016":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
					Bucket: aws.String(name),
					LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: []types.LifecycleRule{{
						ID: aws.String("bad"), Status: types.ExpirationStatusEnabled,
						Filter: &types.LifecycleRuleFilter{Prefix: aws.String("")},
					}}},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-011":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.PutBucketWebsite(ctx, &s3.PutBucketWebsiteInput{
					Bucket: aws.String(name),
					WebsiteConfiguration: &types.WebsiteConfiguration{
						IndexDocument: &types.IndexDocument{Suffix: aws.String("index.html")},
					},
				})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-012":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: aws.String(name)})
				return wrapErr("runGaps", err)
			})
			return callOut, wrapErr("runGaps", callErr)
		}
	case "S3-GAP-014":
		ps := s3.NewPresignClient(api)
		req, err := ps.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(name), Key: aws.String("k")},
			s3.WithPresignExpires(time.Minute))
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("presign: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("runGaps", err)
		}
		resp, err := rt.Clients.Raw.Do(httpReq)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("presign get: %w", err)
		}
		defer closeChecked(resp.Body)
		return cases.Outcome{HTTP: resp.StatusCode, Fields: map[string]string{}}, nil
	case "S3-GAP-015":
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, rt.Cfg.S3Endpoint+"/k", nil)
		if err != nil {
			return cases.Outcome{}, wrapErr("runGaps", err)
		}
		req.Host = name + ".s3." + rt.Region() + ".amazonaws.com"
		if err := v4.NewSigner().SignHTTP(ctx, aws.Credentials{AccessKeyID: rt.Cfg.AccessKey, SecretAccessKey: rt.Cfg.Secret},
			req, harness.PayloadHash(nil), "s3", rt.Region(), time.Now()); err != nil {
			return cases.Outcome{}, fmt.Errorf("sign virtual host: %w", err)
		}
		resp, err := rt.Clients.Raw.Do(req)
		if err != nil {
			return cases.Outcome{}, fmt.Errorf("virtual host: %w", err)
		}
		defer closeChecked(resp.Body)
		return cases.Outcome{HTTP: resp.StatusCode, Fields: map[string]string{}}, nil
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func bucketPolicy(bucket string) string {
	return `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*",` +
		`"Action":"s3:GetObject","Resource":"arn:aws:s3:::` + bucket + `/*"}]}`
}
