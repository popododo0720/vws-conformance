package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

func TestTerminateCleanupChecksCurrentPeers(t *testing.T) {
	for _, row := range []struct {
		name    string
		owner   string
		wantEnd bool
	}{
		{name: "foreign volume", owner: "conf-20261015T130000Z-def456", wantEnd: false},
		{name: "launch device", owner: "", wantEnd: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var mu sync.Mutex
			ended := false
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
				case "TerminateInstances":
					mu.Lock()
					ended = true
					mu.Unlock()
					if _, writeErr := io.WriteString(w, ec2Item("TerminateInstancesResponse",
						"<instancesSet><item><instanceId>i-1</instanceId></item></instancesSet>")); writeErr != nil {
						panic(writeErr)
					}
				case "CreateVpc":
					if _, writeErr := io.WriteString(w, ec2Item("CreateVpcResponse",
						"<vpc><vpcId>vpc-1</vpcId><state>available</state><cidrBlock>10.0.0.0/16</cidrBlock></vpc>")); writeErr != nil {
						panic(writeErr)
					}
				case "CreateSubnet":
					if _, writeErr := io.WriteString(w, ec2Item("CreateSubnetResponse",
						"<subnet><subnetId>subnet-1</subnetId><vpcId>vpc-1</vpcId><state>available</state>"+
							"<cidrBlock>10.0.1.0/24</cidrBlock></subnet>")); writeErr != nil {
						panic(writeErr)
					}
				case "RunInstances":
					if _, writeErr := io.WriteString(w, runWithDevice("i-1", "vol-1")); writeErr != nil {
						panic(writeErr)
					}
				case "DescribeInstances":
					if _, writeErr := io.WriteString(w, describeInstanceXML("i-1", "running")); writeErr != nil {
						panic(writeErr)
					}
				case "DescribeVolumes":
					tags := ""
					if row.owner != "" {
						tags = `<tagSet><item><key>vws:conformance-run</key><value>` + row.owner + `</value></item></tagSet>`
					}
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<DescribeVolumesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><volumeSet><item><volumeId>vol-1</volumeId>`+
						`<status>in-use</status><attachmentSet><item><instanceId>i-1</instanceId>`+
						`<status>attached</status><deleteOnTermination>true</deleteOnTermination></item></attachmentSet>`+
						tags+`</item></volumeSet></DescribeVolumesResponse>`); writeErr != nil {
						panic(writeErr)
					}
				case "DeleteVolume":
					if _, writeErr := io.WriteString(w, ec2Item("DeleteVolumeResponse", "<return>true</return>")); writeErr != nil {
						panic(writeErr)
					}
				default:
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer srv.Close()
			rt := keyRuntime(t, srv.URL)
			_, err := launch(context.Background(), rt, rt.Clients.EC2, "subnet-1", rt.Token("peer"))
			if err != nil && row.wantEnd {
				t.Fatal(err)
			}
			if err := rt.Clean.Run(context.Background()); err != nil && row.wantEnd {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if ended != row.wantEnd {
				t.Fatalf("terminated %v", ended)
			}
		})
	}
}

func runWithDevice(instance, volume string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId><reservationId>r-1</reservationId><ownerId>123456789012</ownerId>` +
		`<instancesSet><item><instanceId>` + instance + `</instanceId>` +
		`<instanceState><code>0</code><name>pending</name></instanceState>` +
		`<blockDeviceMapping><item><deviceName>/dev/sda1</deviceName><ebs>` +
		`<volumeId>` + volume + `</volumeId><status>attached</status>` +
		`<deleteOnTermination>true</deleteOnTermination></ebs></item></blockDeviceMapping>` +
		`</item></instancesSet></RunInstancesResponse>`
}
