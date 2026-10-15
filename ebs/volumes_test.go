package ebs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		body = string(b)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<CreateVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
			`<requestId>r</requestId><volumeId>vol-1</volumeId><size>1</size>`+
			`<availabilityZone>vws-1a</availabilityZone><status>available</status>`+
			`<volumeType>gp2</volumeType></CreateVolumeResponse>`)
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
