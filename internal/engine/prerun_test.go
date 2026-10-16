package engine

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
	"sync"
	"testing"
	"time"

	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/sweep"
)

const stuckVolume = "vol-7d3f00d05104042c9"

func TestForeignLeftoverDoesNotAbortPreRun(t *testing.T) {
	const (
		current = "conf-20261016T080000Z-aabbcc"
		other   = "conf-20261016T071615Z-ce77da"
	)
	cfg := stuckVolumeConfig(t, stuckOpts{current: current, owner: other, ledger: true})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rep, err := runGroups(ctx, cfg, "auth", "sigv4")
	if err != nil {
		t.Fatalf("pre-run aborted: %v", err)
	}
	var reason string
	for _, res := range rep.Results {
		if res.ID == "HARNESS-CLEANUP" && res.Status == "error" {
			reason = res.Reason
		}
	}
	if !strings.Contains(reason, "volume "+stuckVolume) {
		t.Fatalf("HARNESS-CLEANUP reason %q", reason)
	}
}

func TestTaggedForeignVolumeWithoutLedgerDoesNotAbortPreRun(t *testing.T) {
	const (
		current = "conf-20261016T080200Z-ccddee"
		other   = "conf-20261016T071615Z-ce77da"
	)
	cfg := stuckVolumeConfig(t, stuckOpts{current: current, owner: other, tag: true})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rep, err := runGroups(ctx, cfg, "auth", "sigv4")
	if err != nil {
		t.Fatalf("pre-run aborted: %v", err)
	}
	var reason string
	for _, res := range rep.Results {
		if res.ID == "HARNESS-CLEANUP" && res.Status == "error" {
			reason = res.Reason
		}
	}
	if !strings.Contains(reason, "volume "+stuckVolume) {
		t.Fatalf("HARNESS-CLEANUP reason %q", reason)
	}
}

func TestCurrentRunLeftoverStillAbortsPreRun(t *testing.T) {
	const current = "conf-20261016T080100Z-bbccdd"
	cfg := stuckVolumeConfig(t, stuckOpts{current: current, owner: current, ledger: true})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := runGroups(ctx, cfg, "auth", "sigv4")
	if err == nil || !strings.Contains(err.Error(), "pre-run sweep:") || !strings.Contains(err.Error(), stuckVolume) {
		t.Fatalf("pre-run error %v", err)
	}
}

type stuckOpts struct {
	current string
	owner   string
	ledger  bool
	tag     bool
}

func stuckVolumeConfig(t *testing.T, opt stuckOpts) harness.Config {
	t.Helper()
	resetGate(t)
	dir := t.TempDir()
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if opt.owner != opt.current {
		if err := os.WriteFile(filepath.Join(activity, opt.owner), []byte("0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opt.ledger {
		row := sweep.Resource{Kind: "volume", ID: stuckVolume, Name: "conf-vol", LedgerRunID: opt.owner}
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tag := ""
	if opt.tag {
		tag = opt.owner
	}
	srv := httptest.NewServer(stuckVolumeServer(tag))
	t.Cleanup(srv.Close)
	return harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: srv.URL, S3Endpoint: srv.URL, IAMEndpoint: srv.URL,
		AllowedHosts: []string{"127.0.0.1"}, AllowedAccounts: []string{"123456789012"},
		Target: "local-fake", RunID: opt.current, OutDir: dir, Parallel: 1,
		ConsoleAPIVersion: "local",
	}
}

func resetGate(t *testing.T) {
	t.Helper()
	gate.once = sync.Once{}
	gate.rt = nil
	gate.err = nil
	gate.foreign = nil
	published.mu.Lock()
	published.rep = nil
	published.cfg = harness.Config{}
	published.file = ""
	published.mu.Unlock()
	harness.SetProcessCleanup(nil)
	t.Cleanup(func() {
		gate.once = sync.Once{}
		gate.rt = nil
		gate.err = nil
		gate.foreign = nil
		harness.SetProcessCleanup(nil)
	})
}

func stuckVolumeServer(tag string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveStuckVolume(w, r, tag)
	}
}

func serveStuckVolume(w http.ResponseWriter, r *http.Request, tag string) {
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`+
			`<Owner><ID>owner</ID></Owner><Buckets></Buckets></ListAllMyBucketsResult>`)
		return
	}
	action := formAction(r)
	if action == "GetCallerIdentity" {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">`+
			`<GetCallerIdentityResult><Account>123456789012</Account>`+
			`<Arn>arn:aws:iam::123456789012:root</Arn><UserId>AIDEXAMPLE</UserId>`+
			`</GetCallerIdentityResult><ResponseMetadata><RequestId>r</RequestId></ResponseMetadata>`+
			`</GetCallerIdentityResponse>`)
		return
	}
	status, body := stuckEC2(action, tag)
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func formAction(r *http.Request) string {
	if a := r.URL.Query().Get("Action"); a != "" {
		return a
	}
	b, _ := io.ReadAll(r.Body)
	vals, _ := url.ParseQuery(string(b))
	return vals.Get("Action")
}

func stuckEC2(action, tag string) (int, string) {
	const ns = `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`
	errBody := func(code, msg string) (int, string) {
		return 400, `<?xml version="1.0" encoding="UTF-8"?>` +
			`<Response><Errors><Error><Code>` + code + `</Code><Message>` + msg +
			`</Message></Error></Errors><RequestID>r</RequestID></Response>`
	}
	ok := func(body string) (int, string) {
		return 200, `<?xml version="1.0" encoding="UTF-8"?>` + body
	}
	switch action {
	case "DetachVolume":
		return errBody("InvalidAction", "DetachVolume")
	case "DeleteVolume":
		return errBody("IncorrectState", "volume is in use")
	case "DescribeVolumes":
		tags := ""
		if tag != "" {
			tags = `<tagSet><item><key>vws:conformance-run</key><value>` + tag + `</value></item></tagSet>`
		}
		return ok(`<DescribeVolumesResponse ` + ns + `><requestId>r</requestId><volumeSet><item>` +
			`<volumeId>` + stuckVolume + `</volumeId><size>1</size><availabilityZone>vws-1a</availabilityZone>` +
			`<status>in-use</status><volumeType>gp2</volumeType>` + tags +
			`</item></volumeSet></DescribeVolumesResponse>`)
	case "DescribeInstances":
		return ok(`<DescribeInstancesResponse ` + ns + `><requestId>r</requestId>` +
			`<reservationSet/></DescribeInstancesResponse>`)
	case "DescribeSnapshots":
		return ok(`<DescribeSnapshotsResponse ` + ns + `><requestId>r</requestId>` +
			`<snapshotSet/></DescribeSnapshotsResponse>`)
	case "DescribeAddresses":
		return ok(`<DescribeAddressesResponse ` + ns + `><requestId>r</requestId>` +
			`<addressesSet/></DescribeAddressesResponse>`)
	case "DescribeKeyPairs":
		return ok(`<DescribeKeyPairsResponse ` + ns + `><requestId>r</requestId>` +
			`<keySet/></DescribeKeyPairsResponse>`)
	case "DescribeSecurityGroups":
		return ok(`<DescribeSecurityGroupsResponse ` + ns + `><requestId>r</requestId>` +
			`<securityGroupInfo/></DescribeSecurityGroupsResponse>`)
	case "DescribeSubnets":
		return ok(`<DescribeSubnetsResponse ` + ns + `><requestId>r</requestId>` +
			`<subnetSet/></DescribeSubnetsResponse>`)
	case "DescribeVpcs":
		return ok(`<DescribeVpcsResponse ` + ns + `><requestId>r</requestId>` +
			`<vpcSet/></DescribeVpcsResponse>`)
	case "DescribeInternetGateways":
		return ok(`<DescribeInternetGatewaysResponse ` + ns + `><requestId>r</requestId>` +
			`<internetGatewaySet/></DescribeInternetGatewaysResponse>`)
	default:
		return errBody("InvalidAction", action)
	}
}
