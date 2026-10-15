package ebs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMakeVolTagsTheRun(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(string(b), "DescribeVolumes") {
			_, _ = io.WriteString(w, describeVolumeXML("available"))
			return
		}
		body = string(b)
		_, _ = io.WriteString(w, volumeXML("available"))
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{
		RunID: run, OutDir: t.TempDir(), Region: "vws-1",
	}, nil)
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("vws-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "")),
		config.WithEC2IMDSClientEnableState(imds.ClientDisabled),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 1 })
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	api := awsec2.NewFromConfig(awsCfg, func(o *awsec2.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
	if _, err := makeVol(context.Background(), rt, api, "vol"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "vws%3Aconformance-run") && !strings.Contains(body, "vws:conformance-run") {
		t.Fatalf("create body %s", body)
	}
	if !strings.Contains(body, run) {
		t.Fatalf("create body %s", body)
	}
}

func TestMakeVolWaitsUntilAvailable(t *testing.T) {
	var describes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body := string(b)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(body, "DescribeVolumes") {
			n := describes.Add(1)
			status := "creating"
			if n >= 2 {
				status = "available"
			}
			_, _ = io.WriteString(w, describeVolumeXML(status))
			return
		}
		_, _ = io.WriteString(w, volumeXML("creating"))
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{
		RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(), Region: "vws-1",
	}, nil)
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("vws-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "")),
		config.WithEC2IMDSClientEnableState(imds.ClientDisabled),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 1 })
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	api := awsec2.NewFromConfig(awsCfg, func(o *awsec2.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
	})
	id, err := makeVol(context.Background(), rt, api, "vol")
	if err != nil {
		t.Fatal(err)
	}
	if id != "vol-1" || describes.Load() < 2 {
		t.Fatalf("id %s describes %d", id, describes.Load())
	}
}

func volumeXML(status string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<CreateVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId><volumeId>vol-1</volumeId><size>1</size>` +
		`<availabilityZone>vws-1a</availabilityZone><status>` + status + `</status>` +
		`<volumeType>gp2</volumeType></CreateVolumeResponse>`
}

func describeVolumeXML(status string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<DescribeVolumesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId><volumeSet><item><volumeId>vol-1</volumeId><size>1</size>` +
		`<availabilityZone>vws-1a</availabilityZone><status>` + status + `</status>` +
		`<volumeType>gp2</volumeType></item></volumeSet></DescribeVolumesResponse>`
}
