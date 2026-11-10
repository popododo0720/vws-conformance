package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"

	ec2cases "github.com/popododo0720/vws-conformance/ec2"
)

func TestOwnedVolumeTagCasesPassAndCleanUp(t *testing.T) {
	fake, rt := gateRuntime(t)
	group := ec2cases.Tags()
	for i, c := range group.Cases {
		t.Run(c.ID, func(t *testing.T) {
			result := runOne(context.Background(), rt, group, c)
			if result.Status != "pass" {
				t.Fatalf("case result = %+v", result)
			}
			id := fmt.Sprintf("vol-%017d", i+1)
			if _, ok := fake.Volume(id); ok {
				t.Fatalf("case left %s behind", id)
			}
		})
	}
	if fake.Calls("DeleteVolume") != 3 {
		t.Fatalf("cleanup deletions = %d", fake.Calls("DeleteVolume"))
	}
}

func TestTagCaseWaitsForEventualRead(t *testing.T) {
	fake, original := gateRuntime(t)
	var stale atomic.Bool
	var delayed atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		action := r.Form.Get("Action")
		if action == "DescribeVolumes" && stale.Swap(false) {
			delayed.Add(1)
			body := "<DescribeVolumesResponse><volumeSet><item><volumeId>vol-00000000000000001</volumeId>" +
				"<tagSet><item><key>vws-conformance:run</key><value>%s</value></item></tagSet>" +
				"</item></volumeSet></DescribeVolumesResponse>"
			if _, err := fmt.Fprintf(w, body, original.RunID()); err != nil {
				t.Error(err)
			}
			return
		}
		fake.Config.Handler.ServeHTTP(w, r)
		if action == "CreateTags" {
			stale.Store(true)
		}
	}))
	t.Cleanup(proxy.Close)
	cfg := original.Cfg
	cfg.EC2Endpoint = proxy.URL
	clients, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, clients)
	group := ec2cases.Tags()
	result := runOne(context.Background(), rt, group, group.Cases[0])
	if result.Status != "pass" {
		t.Fatalf("delayed tag result = %+v", result)
	}
	if delayed.Load() != 1 {
		t.Fatal("stale response was not exercised")
	}
	if _, ok := fake.Volume("vol-00000000000000001"); ok {
		t.Fatal("delayed read lost cleanup")
	}
}
