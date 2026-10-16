package harness

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

func TestObserveRejectsTheWrongResource(t *testing.T) {
	t.Run("volume", func(t *testing.T) {
		var deletes atomic.Int32
		srv := httptestServer(t, func(action string) (int, string) {
			if action == "DeleteVolume" {
				deletes.Add(1)
				return 200, ec2OK("DeleteVolumeResponse", "<return>true</return>")
			}
			if action == "DescribeVolumes" {
				body := ec2OK("DescribeVolumesResponse",
					"<volumeSet>"+volumeItem("vol-other", "conf-20261015T120000Z-abc123")+"</volumeSet>")
				return 200, body
			}
			return ec2Describe(action, false)
		})
		rt := ec2Runtime(t, srv, t.TempDir())
		err := rt.DeleteOwnedVolume(context.Background(), "vol-1")
		if deletes.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
			t.Fatalf("deletes %d err %v", deletes.Load(), err)
		}
	})
	t.Run("address", func(t *testing.T) {
		var releases atomic.Int32
		srv := httptestServer(t, func(action string) (int, string) {
			if action == "ReleaseAddress" {
				releases.Add(1)
				return 200, ec2OK("ReleaseAddressResponse", "<return>true</return>")
			}
			if action == "DescribeAddresses" {
				body := ec2OK("DescribeAddressesResponse",
					"<addressesSet>"+addressItem("eipalloc-other", "conf-20261015T120000Z-abc123")+"</addressesSet>")
				return 200, body
			}
			return ec2Describe(action, false)
		})
		rt := ec2Runtime(t, srv, t.TempDir())
		err := rt.ReleaseOwned(context.Background(), "eipalloc-1")
		if releases.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
			t.Fatalf("releases %d err %v", releases.Load(), err)
		}
	})
}

func TestVolumePeersReadEveryPage(t *testing.T) {
	var terminates atomic.Int32
	var page atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "TerminateInstances" {
			terminates.Add(1)
			return 200, ec2OK("TerminateInstancesResponse",
				"<instancesSet><item><instanceId>i-1</instanceId></item></instancesSet>")
		}
		if action != "DescribeVolumes" {
			return ec2Describe(action, false)
		}
		n := page.Add(1)
		if n == 1 {
			return 200, ec2OK("DescribeVolumesResponse", "<volumeSet/><nextToken>page-2</nextToken>")
		}
		item := "<item><volumeId>vol-foreign</volumeId><status>in-use</status>" +
			"<attachmentSet><item><instanceId>i-1</instanceId><status>attached</status>" +
			"<deleteOnTermination>true</deleteOnTermination></item></attachmentSet>" +
			tagXML("conf-20261015T130000Z-def456") + "</item>"
		return 200, ec2OK("DescribeVolumesResponse", "<volumeSet>"+item+"</volumeSet>")
	})
	rt := ec2Runtime(t, srv, t.TempDir())
	err := rt.TerminateOwned(context.Background(), "i-1")
	if terminates.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("terminates %d err %v", terminates.Load(), err)
	}
}

func TestConflictingLedgerOwnerIsKept(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var deletes atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "DeleteVolume" || action == "DetachVolume" {
			if action == "DeleteVolume" {
				deletes.Add(1)
			}
			return 200, ec2OK(action+"Response", "<return>true</return>")
		}
		status, body := ownedDescribe(action, nil, nil, map[string]string{"vol-1": run}, nil)
		return status, body
	})
	dir := t.TempDir()
	rows := []sweep.Resource{
		{Kind: "volume", ID: "vol-1", LedgerRunID: run, Target: "local-fake", Account: testAccount},
		{Kind: "volume", ID: "vol-1", LedgerRunID: "not-a-run", Target: "local-fake", Account: testAccount},
	}
	var buf []byte
	for _, row := range rows {
		line, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), buf, 0o644); err != nil {
		t.Fatal(err)
	}
	rt := ec2Runtime(t, srv, dir)
	err := rt.Sweep(context.Background())
	if deletes.Load() != 0 {
		t.Fatalf("deleted vol-1 err %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("err %v", err)
	}
}

func TestKeyTagConflictPreserves(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var deletes atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "DeleteKeyPair" {
			deletes.Add(1)
			return 200, ec2OK("DeleteKeyPairResponse", "<return>true</return>")
		}
		if action == "DescribeKeyPairs" {
			return 200, ec2OK("DescribeKeyPairsResponse",
				"<keySet><item><keyName>"+nameFor(run, "k")+"</keyName>"+
					tagXML("conf-20261015T130000Z-def456")+"</item></keySet>")
		}
		return ec2Describe(action, false)
	})
	rt := ec2Runtime(t, srv, t.TempDir())
	err := rt.DeleteOwnedKey(context.Background(), nameFor(run, "k"))
	if deletes.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("deletes %d err %v", deletes.Load(), err)
	}
}

func nameFor(run, suffix string) string {
	rt := NewRuntime(Config{RunID: run}, nil)
	return rt.Name(suffix)
}

func httptestServer(t *testing.T, fn func(action string) (int, string)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Get("Action") == "" {
			if writeNoSuchTagSet(w, r) {
				return
			}
			writeEC2(w, 200, listBucketsXML(nil))
			return
		}
		status, body := fn(ec2Action(r))
		writeEC2(w, status, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func ec2Runtime(t *testing.T, endpoint, dir string) *Runtime {
	t.Helper()
	return runtimeAt(t, endpoint, dir, "conf-20261015T120000Z-abc123")
}
