package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

func TestSweepReportsVolumeThatStaysInUse(t *testing.T) {
	var deletes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action == "DetachVolume" {
			writeEC2(w, 200, ec2OK("DetachVolumeResponse", "<volumeId>vol-1</volumeId>"))
			return
		}
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

func TestSweepKeepsAnUndetachableVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action == "DetachVolume" {
			writeEC2(w, 400, ec2Err("InvalidAction", "DetachVolume"))
			return
		}
		if action == "DeleteVolume" {
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
	if err == nil || !strings.Contains(err.Error(), "volume vol-1") {
		t.Fatalf("sweep error %v", err)
	}
	ledger, readErr := os.ReadFile(filepath.Join(rt.Cfg.OutDir, "ledger.jsonl"))
	if readErr != nil || !strings.Contains(string(ledger), "vol-1") || !strings.Contains(string(ledger), rt.Cfg.RunID) {
		t.Fatalf("ledger %q err %v", ledger, readErr)
	}
}

func TestSweepLeavesLiveForeignAndOtherTargetVolumes(t *testing.T) {
	const (
		current = "conf-20261015T120000Z-abc123"
		active  = "conf-20261015T130000Z-def456"
	)
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if err := os.WriteFile(filepath.Join(activity, active), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	volumes := map[string]string{
		"vol-foreign": active,
		"vol-reused":  "",
		"vol-mine":    "",
	}
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, listBucketsXML(nil)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		mu.Lock()
		defer mu.Unlock()
		switch vals.Get("Action") {
		case "DeleteVolume":
			id := vals.Get("VolumeId")
			delete(volumes, id)
			deleted["volume "+id] = true
			writeEC2(w, 200, ec2OK("DeleteVolumeResponse", "<return>true</return>"))
		case "DetachVolume":
			writeEC2(w, 200, ec2OK("DetachVolumeResponse", "<volumeId>"+vals.Get("VolumeId")+"</volumeId>"))
		default:
			status, body := ownedDescribe(vals.Get("Action"), nil, nil, volumes, nil)
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	var lines []string
	for _, row := range []sweep.Resource{
		{Kind: "volume", ID: "vol-foreign", LedgerRunID: current, Target: "local-fake"},
		{Kind: "volume", ID: "vol-reused", LedgerRunID: current, Target: "vws-test", Account: "999999999999"},
		{Kind: "volume", ID: "vol-mine", LedgerRunID: current, Target: "local-fake"},
	} {
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := runtimeAt(t, srv.URL, dir, current)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v deleted %v", err, deleted)
	}
	if !deleted["volume vol-mine"] {
		t.Fatal("in-scope untagged volume was kept")
	}
	for _, id := range []string{"volume vol-foreign", "volume vol-reused"} {
		if deleted[id] {
			t.Fatalf("deleted %s", id)
		}
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

func TestSweepKeepsForeignAndActiveResources(t *testing.T) {
	const (
		current = "conf-20261015T120000Z-abc123"
		active  = "conf-20261015T130000Z-def456"
		unknown = "conf-20261015T140000Z-aaa111"
	)
	curName := "conf-20261015t120000z-abc123"
	actName := "conf-20261015t130000z-def456"
	unknownName := "conf-20261015t140000z-aaa111"
	var mu sync.Mutex
	buckets := map[string]bool{
		"conf-logs": true, actName: true, unknownName: true, curName: true,
	}
	groups := map[string]string{
		"sg-bare": "conf-sg", "sg-active": actName, "sg-unknown": unknownName,
	}
	volumes := map[string]string{
		"vol-active": active, "vol-unknown": unknown, "vol-mine": current, "vol-tagged": current,
	}
	addresses := map[string]string{
		"eipalloc-mine": current, "eipalloc-active": active,
	}
	deleted := map[string]bool{}
	dropBucket := func(name string) {
		mu.Lock()
		delete(buckets, name)
		deleted["bucket "+name] = true
		mu.Unlock()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			dropBucket(strings.Trim(r.URL.Path, "/"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			body := listBucketsXML(buckets)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		action := vals.Get("Action")
		mu.Lock()
		defer mu.Unlock()
		switch action {
		case "DeleteVolume":
			id := vals.Get("VolumeId")
			delete(volumes, id)
			deleted["volume "+id] = true
			writeEC2(w, 200, ec2OK("DeleteVolumeResponse", "<return>true</return>"))
		case "ReleaseAddress":
			id := vals.Get("AllocationId")
			delete(addresses, id)
			deleted["address "+id] = true
			writeEC2(w, 200, ec2OK("ReleaseAddressResponse", "<return>true</return>"))
		case "DeleteSecurityGroup":
			id := vals.Get("GroupId")
			delete(groups, id)
			deleted["security-group "+id] = true
			writeEC2(w, 200, ec2OK("DeleteSecurityGroupResponse", "<return>true</return>"))
		case "DetachVolume":
			writeEC2(w, 200, ec2OK("DetachVolumeResponse", "<volumeId>"+vals.Get("VolumeId")+"</volumeId>"))
		default:
			status, body := ownedDescribe(action, buckets, groups, volumes, addresses)
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if err := os.WriteFile(filepath.Join(activity, active), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, row := range []sweep.Resource{
		{Kind: "volume", ID: "vol-active", LedgerRunID: active},
		{Kind: "volume", ID: "vol-unknown", LedgerRunID: unknown},
		{Kind: "volume", ID: "vol-mine", LedgerRunID: current},
	} {
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := runtimeAt(t, srv.URL, dir, current)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v deleted %v", err, deleted)
	}
	wantGone := []string{
		"bucket " + curName,
		"volume vol-mine", "volume vol-tagged",
		"address eipalloc-mine",
	}
	wantKept := []string{
		"bucket conf-logs", "bucket " + actName, "bucket " + unknownName,
		"security-group sg-bare", "security-group sg-active", "security-group sg-unknown",
		"volume vol-active", "volume vol-unknown", "address eipalloc-active",
	}
	for _, id := range wantGone {
		if !deleted[id] {
			t.Errorf("kept %s", id)
		}
	}
	for _, id := range wantKept {
		if deleted[id] {
			t.Errorf("deleted %s", id)
		}
	}
}

func TestSweepKeepsForeignRunAcrossOutDirs(t *testing.T) {
	const (
		mine    = "conf-20261015T120000Z-abc123"
		foreign = "conf-20261015T150000Z-bbb222"
	)
	mineName := "conf-20261015t120000z-abc123"
	foreignName := "conf-20261015t150000z-bbb222"
	var mu sync.Mutex
	buckets := map[string]bool{mineName: true, foreignName: true}
	volumes := map[string]string{"vol-mine": mine, "vol-foreign": foreign}
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path != "/" {
			name := strings.Trim(r.URL.Path, "/")
			mu.Lock()
			delete(buckets, name)
			deleted["bucket "+name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			body := listBucketsXML(buckets)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		action := vals.Get("Action")
		mu.Lock()
		defer mu.Unlock()
		switch action {
		case "DeleteVolume":
			id := vals.Get("VolumeId")
			delete(volumes, id)
			deleted["volume "+id] = true
			writeEC2(w, 200, ec2OK("DeleteVolumeResponse", "<return>true</return>"))
		case "DetachVolume":
			writeEC2(w, 200, ec2OK("DetachVolumeResponse", "<volumeId>"+vals.Get("VolumeId")+"</volumeId>"))
		default:
			status, body := ownedDescribe(action, buckets, nil, volumes, nil)
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	outA := t.TempDir()
	outB := t.TempDir()
	lock := filepath.Join(outA, "active", foreign)
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VWS_CONF_ACTIVITY_DIR", t.TempDir())
	rt := runtimeAt(t, srv.URL, outB, mine)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v deleted %v", err, deleted)
	}
	if deleted["volume vol-foreign"] || deleted["bucket "+foreignName] {
		t.Fatalf("deleted the other run %v", deleted)
	}
	if !deleted["volume vol-mine"] || !deleted["bucket "+mineName] {
		t.Fatalf("kept this run %v", deleted)
	}
}

func TestSweepRespectsLiveMarkerForItsOwnRunID(t *testing.T) {
	const (
		mine = "conf-20261015T120000Z-abc123"
		done = "conf-20261015T170000Z-ddd444"
	)
	mineName := "conf-20261015t120000z-abc123"
	doneName := "conf-20261015t170000z-ddd444"
	var mu sync.Mutex
	buckets := map[string]bool{mineName: true, doneName: true}
	volumes := map[string]string{"vol-mine": mine}
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path != "/" {
			name := strings.Trim(r.URL.Path, "/")
			mu.Lock()
			delete(buckets, name)
			deleted["bucket "+name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			body := listBucketsXML(buckets)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		action := vals.Get("Action")
		mu.Lock()
		defer mu.Unlock()
		switch action {
		case "DeleteVolume":
			id := vals.Get("VolumeId")
			delete(volumes, id)
			deleted["volume "+id] = true
			writeEC2(w, 200, ec2OK("DeleteVolumeResponse", "<return>true</return>"))
		case "DetachVolume":
			writeEC2(w, 200, ec2OK("DetachVolumeResponse", "<volumeId>"+vals.Get("VolumeId")+"</volumeId>"))
		default:
			status, body := ownedDescribe(action, buckets, nil, volumes, nil)
			writeEC2(w, status, body)
		}
	}))
	defer srv.Close()
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if err := os.WriteFile(filepath.Join(activity, mine), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activity, done), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := runtimeAt(t, srv.URL, t.TempDir(), mine)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v deleted %v", err, deleted)
	}
	if deleted["volume vol-mine"] || deleted["bucket "+mineName] {
		t.Fatalf("deleted the live run %v", deleted)
	}
	if !deleted["bucket "+doneName] {
		t.Fatalf("kept a finished run %v", deleted)
	}
}

func TestSweepUsesSharedActivity(t *testing.T) {
	const (
		mine     = "conf-20261015T120000Z-abc123"
		live     = "conf-20261015T130000Z-def456"
		dead     = "conf-20261015T140000Z-aaa111"
		released = "conf-20261015T160000Z-ccc333"
	)
	mineName := "conf-20261015t120000z-abc123"
	liveName := "conf-20261015t130000z-def456"
	deadName := "conf-20261015t140000z-aaa111"
	releasedName := "conf-20261015t160000z-ccc333"
	var mu sync.Mutex
	buckets := map[string]bool{mineName: true, liveName: true, deadName: true, releasedName: true}
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path != "/" {
			name := strings.Trim(r.URL.Path, "/")
			mu.Lock()
			delete(buckets, name)
			deleted["bucket "+name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet {
			mu.Lock()
			body := listBucketsXML(buckets)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		status, body := ownedDescribe(vals.Get("Action"), buckets, nil, nil, nil)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	activity := t.TempDir()
	t.Setenv("VWS_CONF_ACTIVITY_DIR", activity)
	if err := os.WriteFile(filepath.Join(activity, live), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activity, dead), []byte(deadPID(t)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activity, released), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := runtimeAt(t, srv.URL, t.TempDir(), mine)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v deleted %v", err, deleted)
	}
	if deleted["bucket "+liveName] {
		t.Fatalf("deleted a live run %v", deleted)
	}
	if !deleted["bucket "+deadName] || !deleted["bucket "+releasedName] || !deleted["bucket "+mineName] {
		t.Fatalf("kept a finished run %v", deleted)
	}
}

func deadPID(t *testing.T) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(pid)
}

func TestSweepReadsTheSecondBucketPage(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	owned := "conf-20261015t120000z-abc123"
	var mu sync.Mutex
	deleted := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path != "/" {
			mu.Lock()
			deleted["bucket "+strings.Trim(r.URL.Path, "/")] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") {
			token := r.URL.Query().Get("continuation-token")
			names := map[string]bool{"conf-logs": true}
			next := "page-2"
			if token == "page-2" {
				names = map[string]bool{}
				mu.Lock()
				if !deleted["bucket "+owned] {
					names[owned] = true
				}
				mu.Unlock()
				next = ""
			}
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, listBucketsPage(names, next)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, listBucketsXML(nil)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		status, body := ec2Describe(vals.Get("Action"), false)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeAt(t, srv.URL, t.TempDir(), run)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if !deleted["bucket "+owned] {
		t.Fatal("owned bucket on page 2 was kept")
	}
	if deleted["bucket conf-logs"] {
		t.Fatal("deleted conf-logs")
	}
}

func TestSweepReadsTheSecondDescribePage(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var page atomic.Int32
	var terminated atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			if _, writeErr := io.WriteString(w, listBucketsXML(nil)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		vals := ec2Form(r)
		if vals.Get("Action") == "TerminateInstances" {
			terminated.Store(true)
			writeEC2(w, 200, ec2OK("TerminateInstancesResponse", "<instancesSet/>"))
			return
		}
		if vals.Get("Action") == "DescribeInstances" {
			if vals.Get("NextToken") == "" {
				page.Add(1)
				writeEC2(w, 200, ec2OK("DescribeInstancesResponse", "<reservationSet/><nextToken>page-2</nextToken>"))
				return
			}
			if terminated.Load() {
				writeEC2(w, 200, ec2OK("DescribeInstancesResponse", "<reservationSet/>"))
				return
			}
			item := "<item><instanceId>i-page2</instanceId>" +
				"<instanceState><code>16</code><name>running</name></instanceState>" +
				tagXML(run) + "</item>"
			inner := "<reservationSet><item><reservationId>r-1</reservationId>" +
				"<ownerId>123456789012</ownerId><instancesSet>" + item +
				"</instancesSet></item></reservationSet>"
			writeEC2(w, 200, ec2OK("DescribeInstancesResponse", inner))
			return
		}
		status, body := ec2Describe(vals.Get("Action"), false)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := runtimeAt(t, srv.URL, t.TempDir(), run)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rt.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if !terminated.Load() {
		t.Fatal("instance on page 2 was kept")
	}
}

func listBucketsXML(names map[string]bool) string {
	return listBucketsPage(names, "")
}

func listBucketsPage(names map[string]bool, token string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	b.WriteString(`<Owner><ID>id</ID><DisplayName>name</DisplayName></Owner><Buckets>`)
	for name := range names {
		b.WriteString(`<Bucket><Name>` + name + `</Name><CreationDate>2020-01-01T00:00:00.000Z</CreationDate></Bucket>`)
	}
	b.WriteString(`</Buckets>`)
	if token != "" {
		b.WriteString(`<ContinuationToken>` + token + `</ContinuationToken>`)
	}
	b.WriteString(`</ListAllMyBucketsResult>`)
	return b.String()
}

func ec2OK(root, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<` + root + ` xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId>` + inner + `</` + root + `>`
}

func ownedDescribe(action string, buckets map[string]bool, groups, volumes, addresses map[string]string) (int, string) {
	_ = buckets
	switch action {
	case "DescribeInstances":
		return 200, ec2OK("DescribeInstancesResponse", "<reservationSet/>")
	case "DescribeVolumes":
		var items string
		for id, run := range volumes {
			items += volumeItem(id, run)
		}
		return 200, ec2OK("DescribeVolumesResponse", "<volumeSet>"+items+"</volumeSet>")
	case "DescribeSnapshots":
		return 200, ec2OK("DescribeSnapshotsResponse", "<snapshotSet/>")
	case "DescribeAddresses":
		var items string
		for id, run := range addresses {
			items += addressItem(id, run)
		}
		return 200, ec2OK("DescribeAddressesResponse", "<addressesSet>"+items+"</addressesSet>")
	case "DescribeKeyPairs":
		return 200, ec2OK("DescribeKeyPairsResponse", "<keySet/>")
	case "DescribeSecurityGroups":
		var items string
		for id, name := range groups {
			items += groupItem(id, name)
		}
		inner := "<securityGroupInfo>" + items + "</securityGroupInfo>"
		return 200, ec2OK("DescribeSecurityGroupsResponse", inner)
	case "DescribeSubnets":
		return 200, ec2OK("DescribeSubnetsResponse", "<subnetSet/>")
	case "DescribeVpcs":
		return 200, ec2OK("DescribeVpcsResponse", "<vpcSet/>")
	case "DescribeInternetGateways":
		return 400, ec2Err("InvalidAction", "DescribeInternetGateways")
	default:
		return 400, ec2Err("InvalidAction", action)
	}
}

func volumeItem(id, run string) string {
	return "<item><volumeId>" + id + "</volumeId>" +
		"<size>1</size><status>available</status><volumeType>gp2</volumeType>" +
		"<availabilityZone>vws-1a</availabilityZone>" + tagXML(run) + "</item>"
}

func addressItem(id, run string) string {
	return "<item><allocationId>" + id + "</allocationId>" +
		"<domain>vpc</domain><publicIp>203.0.113.10</publicIp>" +
		tagXML(run) + "</item>"
}

func groupItem(id, name string) string {
	return "<item><groupId>" + id + "</groupId><groupName>" + name + "</groupName>" +
		"<groupDescription>d</groupDescription><ownerId>123456789012</ownerId></item>"
}

func tagXML(run string) string {
	return `<tagSet><item><key>vws:conformance-run</key><value>` + run + `</value></item></tagSet>`
}

func ec2Form(r *http.Request) url.Values {
	if a := r.URL.Query().Get("Action"); a != "" {
		return r.URL.Query()
	}
	b, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		panic(readErr)
	}
	vals, queryErr := url.ParseQuery(string(b))
	if queryErr != nil {
		panic(queryErr)
	}
	return vals
}

func runtimeAt(t *testing.T, endpoint, dir, runID string) *Runtime {
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
	ec2c := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) { o.BaseEndpoint = aws.String(endpoint) })
	s3c := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return NewRuntime(Config{
		Region: "vws-1", OutDir: dir, RunID: runID, Target: "local-fake",
	}, &Clients{EC2: ec2c, S3: s3c})
}

func runtimeOn(t *testing.T, endpoint, volume string) *Runtime {
	t.Helper()
	dir := t.TempDir()
	row := sweep.Resource{
		Kind: "volume", ID: volume, Name: "conf-vol",
		LedgerRunID: "conf-20261015T120000Z-abc123",
		Target:      "local-fake",
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
	b, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		panic(readErr)
	}
	vals, queryErr := url.ParseQuery(string(b))
	if queryErr != nil {
		panic(queryErr)
	}
	return vals.Get("Action")
}

func writeEC2(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	if _, writeErr := io.WriteString(w, body); writeErr != nil {
		panic(writeErr)
	}
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
