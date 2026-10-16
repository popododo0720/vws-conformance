package s3

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func bytesReader(s string) *bytes.Reader { return bytes.NewReader([]byte(s)) }

func bucketName(rt *harness.Runtime, id string) string {
	return strings.ToLower(rt.Name(id))
}

func deleteAllObjects(ctx context.Context, api *s3.Client, name string) error {
	var token *string
	seen := map[string]bool{}
	for {
		page, err := api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(name), ContinuationToken: token,
		})
		if err != nil {
			if ignoreNoSuch(err) == nil {
				return nil
			}
			return fmt.Errorf("list %s: %w", name, err)
		}
		for _, obj := range page.Contents {
			_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(name), Key: obj.Key,
			})
			if err != nil {
				return fmt.Errorf("delete %s: %w", aws.ToString(obj.Key), err)
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		next := aws.ToString(page.NextContinuationToken)
		if next == "" || seen[next] {
			return fmt.Errorf("repeated pagination token %q", next)
		}
		seen[next] = true
		token = aws.String(next)
	}
	return deleteVersions(ctx, api, name)
}

func deleteVersions(ctx context.Context, api *s3.Client, name string) error {
	var keyMarker, versionMarker *string
	seen := map[string]bool{}
	for {
		page, err := api.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket: aws.String(name), KeyMarker: keyMarker, VersionIdMarker: versionMarker,
		})
		if err != nil {
			if ignoreNoSuch(err) == nil || versionListUnsupported(err) {
				return nil
			}
			return fmt.Errorf("list versions %s: %w", name, err)
		}
		for _, obj := range page.Versions {
			if err := deleteVersion(ctx, api, name, obj.Key, obj.VersionId); err != nil {
				return err
			}
		}
		for _, obj := range page.DeleteMarkers {
			if err := deleteVersion(ctx, api, name, obj.Key, obj.VersionId); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			return nil
		}
		next := aws.ToString(page.NextKeyMarker) + "\n" + aws.ToString(page.NextVersionIdMarker)
		if next == "\n" || seen[next] {
			return fmt.Errorf("repeated pagination token %q", next)
		}
		seen[next] = true
		keyMarker = page.NextKeyMarker
		versionMarker = page.NextVersionIdMarker
	}
}

func deleteVersion(ctx context.Context, api *s3.Client, name string, key, version *string) error {
	_, err := api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(name), Key: key, VersionId: version,
	})
	if err != nil {
		return fmt.Errorf("delete %s: %w", aws.ToString(key), err)
	}
	return nil
}

func versionListUnsupported(err error) bool {
	s := err.Error()
	return strings.Contains(s, "NotImplemented") || strings.Contains(s, "not served") || strings.Contains(s, "501")
}

func ignoreNoSuch(err error) error {
	if err == nil || strings.Contains(err.Error(), "NoSuch") || strings.Contains(err.Error(), "NotFound") {
		return nil
	}
	return err
}

func createFresh(ctx context.Context, rt *harness.Runtime, api *s3.Client, name string) error {
	_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
	if err != nil {
		return fmt.Errorf("create bucket %s: %w", name, err)
	}
	if !harness.AllowMutation(true, nil, nil, func(string) bool { return false }) {
		return fmt.Errorf("preserve bucket %s", name)
	}
	rt.Cleanup(func(ctx context.Context) error {
		return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
			if err := deleteAllObjects(ctx, api, name); err != nil {
				return err
			}
			_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
			return ignoreNoSuch(err)
		})
	})
	if err := rt.Track("bucket", name, name); err != nil {
		return wrapErr("createFresh", err)
	}
	return nil
}
