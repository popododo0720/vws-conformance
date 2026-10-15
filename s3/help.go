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
			return nil
		}
		next := aws.ToString(page.NextContinuationToken)
		if next == "" || seen[next] {
			return fmt.Errorf("repeated pagination token %q", next)
		}
		seen[next] = true
		token = aws.String(next)
	}
}

func ignoreNoSuch(err error) error {
	if err == nil || strings.Contains(err.Error(), "NoSuch") || strings.Contains(err.Error(), "NotFound") {
		return nil
	}
	return err
}

func createFresh(ctx context.Context, rt *harness.Runtime, api *s3.Client, name string) error {
	if err := rt.Track("bucket", name, name); err != nil {
		return err
	}
	_, err := api.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(name)})
	if err != nil {
		return fmt.Errorf("create bucket %s: %w", name, err)
	}
	rt.Cleanup(func(ctx context.Context) error {
		if err := deleteAllObjects(ctx, api, name); err != nil {
			return err
		}
		_, err := api.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
		return ignoreNoSuch(err)
	})
	return nil
}
