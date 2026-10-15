package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

func TestSweepReportsVolumeThatStaysInUse(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action == "DeleteVolume" {
			deletes.Add(1)
			writeEC2(w, 400, ec2Err("IncorrectState", "volume is in use"))
			return
		}
		status, body := ec2Describe(action, true)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeOn(t, srv.URL, "vol-1")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := rt.Sweep(ctx)
	if err == nil || !strings.Contains(err.Error(), "vol-1") {
		t.Fatalf("sweep error %v", err)
	}
	if deletes.Load() < 2 {
		t.Fatalf("deletes %d", deletes.Load())
	}
}

func TestSweepDetachesAnAttachedVolume(t *testing.T) {
	var detached atomic.Bool
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		switch action {
		case "DetachVolume":
			detached.Store(true)
			writeEC2(w, 200, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DetachVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><volumeId>vol-1</volumeId><status>detaching</status>`+
				`</DetachVolumeResponse>`)
		case "DeleteVolume":
			deletes.Add(1)
			if !detached.Load() {
				writeEC2(w, 400, ec2Err("IncorrectState", "volume is in use"))
				return
			}
			writeEC2(w, 200, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DeleteVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><return>true</return></DeleteVolumeResponse>`)
		default:
			status, body := ec2Describe(action, !detached.Load())
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	rt := runtimeOn(t, srv.URL, "vol-1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if !detached.Load() || deletes.Load() < 1 {
		t.Fatalf("detached %v deletes %d", detached.Load(), deletes.Load())
	}
}

func TestSweepSucceedsWhenVolumeDisappears(t *testing.T) {
	var gone atomic.Bool
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action == "DeleteVolume" {
			deletes.Add(1)
			gone.Store(true)
			writeEC2(w, 200, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DeleteVolumeResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><return>true</return></DeleteVolumeResponse>`)
			return
		}
		status, body := ec2Describe(action, !gone.Load())
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeOn(t, srv.URL, "vol-1")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() < 1 {
		t.Fatalf("deletes %d", deletes.Load())
	}
}

func runtimeOn(t *testing.T, endpoint, volume string) *Runtime {
	t.Helper()
	dir := t.TempDir()
	row := sweep.Resource{
		Kind: "volume", ID: volume, Name: "conf-vol",
		LedgerRunID: "conf-20261015T120000Z-abc123",
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
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
	client := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	return NewRuntime(Config{
		Region: "vws-1", OutDir: dir, RunID: row.LedgerRunID, Target: "local-fake",
	}, &Clients{EC2: client})
}

func ec2Action(r *http.Request) string {
	if a := r.URL.Query().Get("Action"); a != "" {
		return a
	}
	b, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(b))
	return vals.Get("Action")
}

func writeEC2(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func ec2Err(code, msg string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Response><Errors><Error><Code>` + code + `</Code><Message>` + msg +
		`</Message></Error></Errors><RequestID>r</RequestID></Response>`
}

func ec2Describe(action string, volume bool) (int, string) {
	const ns = `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`
	ok := func(body string) (int, string) {
		return 200, `<?xml version="1.0" encoding="UTF-8"?>` + body
	}
	switch action {
	case "DescribeInstances":
		body := `<DescribeInstancesResponse ` + ns + `><requestId>r</requestId><reservationSet/></DescribeInstancesResponse>`
		return ok(body)
	case "DescribeVolumes":
		if !volume {
			return ok(`<DescribeVolumesResponse ` + ns + `><requestId>r</requestId><volumeSet/></DescribeVolumesResponse>`)
		}
		return ok(`<DescribeVolumesResponse ` + ns + `><requestId>r</requestId><volumeSet><item>` +
			`<volumeId>vol-1</volumeId><size>1</size><availabilityZone>vws-1a</availabilityZone>` +
			`<status>in-use</status><volumeType>gp2</volumeType></item></volumeSet></DescribeVolumesResponse>`)
	case "DescribeSnapshots":
		return ok(`<DescribeSnapshotsResponse ` + ns + `><requestId>r</requestId><snapshotSet/></DescribeSnapshotsResponse>`)
	case "DescribeAddresses":
		return ok(`<DescribeAddressesResponse ` + ns + `><requestId>r</requestId><addressesSet/></DescribeAddressesResponse>`)
	case "DescribeKeyPairs":
		return ok(`<DescribeKeyPairsResponse ` + ns + `><requestId>r</requestId><keySet/></DescribeKeyPairsResponse>`)
	case "DescribeSecurityGroups":
		body := `<DescribeSecurityGroupsResponse ` + ns + `>` +
			`<requestId>r</requestId><securityGroupInfo/></DescribeSecurityGroupsResponse>`
		return ok(body)
	case "DescribeSubnets":
		return ok(`<DescribeSubnetsResponse ` + ns + `><requestId>r</requestId><subnetSet/></DescribeSubnetsResponse>`)
	case "DescribeVpcs":
		return ok(`<DescribeVpcsResponse ` + ns + `><requestId>r</requestId><vpcSet/></DescribeVpcsResponse>`)
	case "DescribeInternetGateways":
		return 400, ec2Err("InvalidAction", "DescribeInternetGateways")
	default:
		return 400, ec2Err("InvalidAction", action)
	}
}
