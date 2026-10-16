package ebs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestUnexpectedCreateIsTracked(t *testing.T) {
	for _, row := range []struct {
		id    string
		clean string
	}{
		{id: "EBS-VOL-103", clean: "DeleteVolume"},
		{id: "EBS-SNAP-103", clean: "DeleteSnapshot"},
	} {
		t.Run(row.id, func(t *testing.T) {
			var mu sync.Mutex
			cleaned := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					panic(err)
				}
				vals, err := url.ParseQuery(string(body))
				if err != nil {
					panic(err)
				}
				action := vals.Get("Action")
				w.Header().Set("Content-Type", "text/xml")
				switch action {
				case "DeleteVolume", "DeleteSnapshot":
					if action == row.clean {
						mu.Lock()
						cleaned = true
						mu.Unlock()
					}
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<`+action+`Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><return>true</return></`+action+`Response>`); writeErr != nil {
						panic(writeErr)
					}
				case "CreateVolume":
					if _, writeErr := io.WriteString(w, volumeXML("available")); writeErr != nil {
						panic(writeErr)
					}
				case "DescribeVolumes":
					if _, writeErr := io.WriteString(w, describeVolumeXML("available")); writeErr != nil {
						panic(writeErr)
					}
				case "CreateSnapshot":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<CreateSnapshotResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><snapshotId>snap-leak</snapshotId><volumeId>vol-1</volumeId>`+
						`<status>completed</status><ownerId>123456789012</ownerId></CreateSnapshotResponse>`); writeErr != nil {
						panic(writeErr)
					}
				default:
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()
			cfg := harness.Config{
				AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
				EC2Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
				AllowedAccounts: []string{"123456789012"},
				Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123",
				OutDir: t.TempDir(), AccountID: "123456789012",
			}
			cl, err := harness.Load(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			rt := harness.NewRuntime(cfg, cl)
			var spec cases.Case
			group := Volumes().Cases
			run := runVol
			if row.id == "EBS-SNAP-103" {
				group = Snapshots().Cases
				run = runSnap
			}
			for _, c := range group {
				if c.ID == row.id {
					spec = c
				}
			}
			if _, err := run(context.Background(), rt, spec); err != nil {
				t.Fatal(err)
			}
			if err := rt.Clean.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if !cleaned {
				t.Fatalf("%s left the created resource", row.id)
			}
		})
	}
}
