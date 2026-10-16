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

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestMakeVolRetriesWhenTagsAreUnsupported(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var tagged, plain atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		body := string(b)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(body, "DescribeVolumes") {
			if _, writeErr := io.WriteString(w, describeVolumeXML("available")); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if strings.Contains(body, "TagSpecification") {
			tagged.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidParameterValue</Code>`+
				`<Message>unsupported</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		plain.Add(1)
		if _, writeErr := io.WriteString(w, volumeXML("available")); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{RunID: run, OutDir: t.TempDir(), Region: "vws-1"}, nil)
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
	api := awsec2.NewFromConfig(awsCfg, func(o *awsec2.Options) { o.BaseEndpoint = aws.String(srv.URL) })
	id, err := makeVol(context.Background(), rt, api, "vol")
	if err != nil || id != "vol-1" || tagged.Load() != 1 || plain.Load() != 1 {
		t.Fatalf("id %s err %v tagged %d plain %d", id, err, tagged.Load(), plain.Load())
	}
}

func TestCreateVolumeRecordsTheTaggedRejection(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var tagged, plain atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		body := string(b)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(body, "TagSpecification") {
			tagged.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidParameterValue</Code>`+
				`<Message>unsupported</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		plain.Add(1)
		if _, writeErr := io.WriteString(w, strings.Replace(volumeXML("available"), "vol-1", "vol-0123456789abcdef0",
			1)); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: run, OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	var spec cases.Case
	for _, c := range Volumes().Cases {
		if c.ID == "EBS-VOL-001" {
			spec = c
		}
	}
	got, err := runVol(context.Background(), rt, spec)
	status, reason := expect.Judge(spec.Expect, got)
	if err != nil || status != "fail" || got.HTTP != 400 || got.Code != "InvalidParameterValue" ||
		plain.Load() != 0 || tagged.Load() != 1 || reason != `http got 400 want 200, code "InvalidParameterValue"` {
		t.Fatalf("status %s reason %s http %d code %s err %v tagged %d plain %d",
			status, reason, got.HTTP, got.Code, err, tagged.Load(), plain.Load())
	}
}

func TestMakeVolTagsTheRun(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(string(b), "DescribeVolumes") {
			if _, writeErr := io.WriteString(w, describeVolumeXML("available")); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		body = string(b)
		if _, writeErr := io.WriteString(w, volumeXML("available")); writeErr != nil {
			panic(writeErr)
		}
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
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		body := string(b)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(body, "DescribeVolumes") {
			n := describes.Add(1)
			status := "creating"
			if n >= 2 {
				status = "available"
			}
			if _, writeErr := io.WriteString(w, describeVolumeXML(status)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if _, writeErr := io.WriteString(w, volumeXML("creating")); writeErr != nil {
			panic(writeErr)
		}
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
