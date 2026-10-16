package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func TestAllocIDRetriesWhenTagsAreUnsupported(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var tagged, plain atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(string(b), "TagSpecification") {
			tagged.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidParameterValue</Code>`+
				`<Message>unsupported</Message></Error></Errors><RequestID>r</RequestID></Response>`)
			return
		}
		plain.Add(1)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<AllocateAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
			`<requestId>r</requestId><publicIp>203.0.113.10</publicIp><domain>vpc</domain>`+
			`<allocationId>eipalloc-1</allocationId></AllocateAddressResponse>`)
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{RunID: run, OutDir: t.TempDir(), Region: "vws-1"}, nil)
	id, err := allocID(context.Background(), rt, testEC2(t, srv.URL))
	if err != nil || id != "eipalloc-1" || tagged.Load() != 1 || plain.Load() != 1 {
		t.Fatalf("id %s err %v tagged %d plain %d", id, err, tagged.Load(), plain.Load())
	}
}

func TestAllocateAddressRecordsTheTaggedRejection(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var tagged, plain atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		if strings.Contains(string(b), "TagSpecification") {
			tagged.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidParameterValue</Code>`+
				`<Message>unsupported</Message></Error></Errors><RequestID>r</RequestID></Response>`)
			return
		}
		plain.Add(1)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<AllocateAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
			`<requestId>r</requestId><publicIp>203.0.113.10</publicIp><domain>vpc</domain>`+
			`<allocationId>eipalloc-0123456789abcdef0</allocationId></AllocateAddressResponse>`)
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
	for _, c := range Addresses().Cases {
		if c.ID == "EC2-EIP-001" {
			spec = c
		}
	}
	got, err := alloc(context.Background(), rt, cl.EC2)
	status, reason := expect.Judge(spec.Expect, got)
	if err != nil || status != "fail" || got.HTTP != 400 || got.Code != "InvalidParameterValue" ||
		plain.Load() != 0 || tagged.Load() != 1 || reason != `http got 400 want 200, code "InvalidParameterValue"` {
		t.Fatalf("status %s reason %s http %d code %s err %v tagged %d plain %d",
			status, reason, got.HTTP, got.Code, err, tagged.Load(), plain.Load())
	}
}

func TestAllocIDReleasesWhenLedgerFails(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var body string
	var releases atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		w.Header().Set("Content-Type", "text/xml")
		if vals.Get("Action") == "ReleaseAddress" {
			releases.Add(1)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<ReleaseAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><return>true</return></ReleaseAddressResponse>`)
			return
		}
		body = string(b)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<AllocateAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
			`<requestId>r</requestId><publicIp>203.0.113.10</publicIp><domain>vpc</domain>`+
			`<allocationId>eipalloc-1</allocationId></AllocateAddressResponse>`)
	}))
	defer srv.Close()
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(harness.Config{RunID: run, OutDir: blocked, Region: "vws-1"}, nil)
	api := testEC2(t, srv.URL)
	_, err := allocID(context.Background(), rt, api)
	if err == nil {
		t.Fatal("ledger accepted a file as the output directory")
	}
	if cerr := rt.Clean.Run(context.Background()); cerr != nil {
		t.Fatal(cerr)
	}
	if releases.Load() != 1 {
		t.Errorf("releases %d err %v", releases.Load(), err)
	}
	if !strings.Contains(body, "vws%3Aconformance-run") && !strings.Contains(body, "vws:conformance-run") {
		t.Errorf("allocate body %s", body)
	}
	if !strings.Contains(body, run) {
		t.Errorf("allocate body %s", body)
	}
}

func testEC2(t *testing.T, endpoint string) *awsec2.Client {
	t.Helper()
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
	return awsec2.NewFromConfig(awsCfg, func(o *awsec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
}
