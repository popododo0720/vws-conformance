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

func TestAttachFailureIsTheCaseOutcome(t *testing.T) {
	for _, id := range []string{"EBS-VOL-006", "EBS-VOL-102"} {
		t.Run(id, func(t *testing.T) {
			var mu sync.Mutex
			followed := []string{}
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
				case "DetachVolume", "DeleteVolume":
					mu.Lock()
					followed = append(followed, action)
					mu.Unlock()
					w.WriteHeader(http.StatusOK)
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<`+action+`Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><return>true</return></`+action+`Response>`); writeErr != nil {
						panic(writeErr)
					}
				case "AttachVolume":
					code := "IncorrectState"
					if id == "EBS-VOL-102" {
						code = "VolumeInUse"
					}
					w.WriteHeader(http.StatusBadRequest)
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<Response><Errors><Error><Code>`+code+`</Code>`+
						`<Message>setup failed</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
						panic(writeErr)
					}
				case "CreateVpc":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<CreateVpcResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><vpc><vpcId>vpc-1</vpcId><state>available</state>`+
						`<cidrBlock>10.0.0.0/16</cidrBlock></vpc></CreateVpcResponse>`); writeErr != nil {
						panic(writeErr)
					}
				case "DescribeVpcs":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<DescribeVpcsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><vpcSet><item><vpcId>vpc-1</vpcId><state>available</state>`+
						`<cidrBlock>10.0.0.0/16</cidrBlock>`+
						`<tagSet><item><key>vws:conformance-run</key>`+
						`<value>conf-20261015T120000Z-abc123</value></item></tagSet>`+
						`</item></vpcSet></DescribeVpcsResponse>`); writeErr != nil {
						panic(writeErr)
					}
				case "CreateSubnet":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<CreateSubnetResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><subnet><subnetId>subnet-1</subnetId><vpcId>vpc-1</vpcId>`+
						`<state>available</state><cidrBlock>10.0.1.0/24</cidrBlock>`+
						`</subnet></CreateSubnetResponse>`); writeErr != nil {
						panic(writeErr)
					}
				case "RunInstances":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><reservationId>r-1</reservationId><ownerId>123456789012</ownerId>`+
						`<instancesSet><item><instanceId>i-1</instanceId>`+
						`<instanceState><code>0</code><name>pending</name></instanceState>`+
						`</item></instancesSet></RunInstancesResponse>`); writeErr != nil {
						panic(writeErr)
					}
				case "DescribeInstances":
					if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
						`<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
						`<requestId>r</requestId><reservationSet><item><instancesSet><item>`+
						`<instanceId>i-1</instanceId><instanceState><code>16</code><name>running</name></instanceState>`+
						`</item></instancesSet></item></reservationSet></DescribeInstancesResponse>`); writeErr != nil {
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
				Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123",
				OutDir: t.TempDir(), AccountID: "123456789012",
			}
			cl, err := harness.Load(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			rt := harness.NewRuntime(cfg, cl)
			var spec cases.Case
			for _, c := range Volumes().Cases {
				if c.ID == id {
					spec = c
				}
			}
			got, err := runVol(context.Background(), rt, spec)
			mu.Lock()
			defer mu.Unlock()
			if len(followed) != 0 {
				t.Fatalf("http %d code %q err %v followed %v", got.HTTP, got.Code, err, followed)
			}
			matches := got.HTTP == spec.Expect.HTTP && got.Code == spec.Expect.Code
			if matches && err == nil {
				t.Fatalf("setup outcome satisfied %s", id)
			}
			if !matches && err != nil {
				t.Fatalf("http %d code %q err %v", got.HTTP, got.Code, err)
			}
		})
	}
}
