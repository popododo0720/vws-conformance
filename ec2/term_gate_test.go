package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestTerminateCaseChecksCurrentPeers(t *testing.T) {
	var mu sync.Mutex
	terminated := false
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
			terminated = true
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
		case "DescribeVpcs":
			if _, writeErr := io.WriteString(w, ec2Item("DescribeVpcsResponse", ownedVpcSet())); writeErr != nil {
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
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DescribeVolumesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><volumeSet><item><volumeId>vol-1</volumeId>`+
				`<status>in-use</status><attachmentSet><item><instanceId>i-1</instanceId>`+
				`<status>attached</status><deleteOnTermination>true</deleteOnTermination></item></attachmentSet>`+
				`<tagSet><item><key>vws:conformance-run</key><value>conf-20261015T130000Z-def456</value></item></tagSet>`+
				`</item></volumeSet></DescribeVolumesResponse>`); writeErr != nil {
				panic(writeErr)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	rt := keyRuntime(t, srv.URL)
	var spec cases.Case
	for _, c := range Instances().Cases {
		if c.ID == "EC2-TERM-001" {
			spec = c
		}
	}
	if _, err := runInstances(context.Background(), rt, spec); err == nil {
		t.Fatal("terminate proceeded without a verified peer")
	}
	mu.Lock()
	defer mu.Unlock()
	if terminated {
		t.Fatal("TerminateInstances ran with a foreign DeleteOnTermination volume")
	}
}
