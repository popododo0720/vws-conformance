package ebs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
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

func TestMakeVolReturnsLedgerErrorAndKeepsCleanup(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		w.Header().Set("Content-Type", "text/xml")
		body := string(b)
		switch {
		case strings.Contains(body, "DescribeVolumes"):
			if _, writeErr := io.WriteString(w, describeVolumeXML("available")); writeErr != nil {
				panic(writeErr)
			}
		case strings.Contains(body, "DeleteVolume"):
			deletes.Add(1)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DeleteVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><return>true</return></DeleteVolumeResponse>`); writeErr != nil {
				panic(writeErr)
			}
		default:
			if _, writeErr := io.WriteString(w, volumeXML("available")); writeErr != nil {
				panic(writeErr)
			}
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	blocked := dir + "/not-a-directory"
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(harness.Config{
		RunID: run, OutDir: blocked, Region: "vws-1", Target: "local-fake", AccountID: "123456789012",
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
	api := awsec2.NewFromConfig(awsCfg, func(o *awsec2.Options) { o.BaseEndpoint = aws.String(srv.URL) })
	if _, err := makeVol(context.Background(), rt, api, "vol"); err == nil || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("err %v", err)
	}
	if err := rt.Clean.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "read ledger") {
		t.Fatalf("cleanup %v", err)
	}
	if deletes.Load() != 0 {
		t.Fatalf("deletes %d", deletes.Load())
	}
}

func TestDeleteMissingVolumeLeavesThePlantedID(t *testing.T) {
	const (
		planted = "vol-0123456789abcdef0"
		owned   = "vol-ownedgone0000001"
		run     = "conf-20261016T090000Z-aabbcc"
	)
	var mu sync.Mutex
	var deleted []string
	gone := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		vals, queryErr := url.ParseQuery(string(b))
		if queryErr != nil {
			panic(queryErr)
		}
		action := vals.Get("Action")
		w.Header().Set("Content-Type", "text/xml")
		const ns = `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`
		notFound := func() {
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidVolume.NotFound</Code>`+
				`<Message>volume is gone</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
				panic(writeErr)
			}
		}
		switch action {
		case "CreateVolume":
			if _, writeErr := io.WriteString(w, strings.Replace(volumeXML("available"), "vol-1", owned, 1)); writeErr != nil {
				panic(writeErr)
			}
		case "DescribeVolumes":
			id := vals.Get("VolumeId.1")
			mu.Lock()
			missing := gone[id]
			mu.Unlock()
			if id == "" || missing {
				notFound()
				return
			}
			body := strings.Replace(describeVolumeXML("available"), "vol-1", id, 1)
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
		case "DeleteVolume":
			id := vals.Get("VolumeId")
			mu.Lock()
			deleted = append(deleted, id)
			already := gone[id]
			gone[id] = true
			mu.Unlock()
			if already {
				notFound()
				return
			}
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DeleteVolumeResponse `+ns+`><requestId>r</requestId>`+
				`<return>true</return></DeleteVolumeResponse>`); writeErr != nil {
				panic(writeErr)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>InvalidAction</Code><Message>`+action+
				`</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
				panic(writeErr)
			}
		}
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: run, OutDir: t.TempDir(), AccountID: "123456789012",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	var spec cases.Case
	for _, c := range Volumes().Cases {
		if c.ID == "EBS-VOL-101" {
			spec = c
		}
	}
	got, err := runVol(context.Background(), rt, spec)
	mu.Lock()
	defer mu.Unlock()
	for _, id := range deleted {
		if id == planted {
			t.Fatalf("deleted planted %s outcome http %d code %q err %v", planted, got.HTTP, got.Code, err)
		}
	}
	if err != nil || got.HTTP != 400 || got.Code != "InvalidVolume.NotFound" {
		t.Fatalf("http %d code %q err %v deleted %v", got.HTTP, got.Code, err, deleted)
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
