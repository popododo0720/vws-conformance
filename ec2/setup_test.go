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
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestAssociateFailureIsTheCaseOutcome(t *testing.T) {
	var mu sync.Mutex
	followed := false
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
		case "DisassociateAddress":
			mu.Lock()
			followed = true
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			body := ec2Item("DisassociateAddressResponse", "<return>true</return>")
			if _, writeErr := io.WriteString(w, body); writeErr != nil {
				panic(writeErr)
			}
		case "AssociateAddress":
			w.WriteHeader(http.StatusBadRequest)
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<Response><Errors><Error><Code>IncorrectState</Code>`+
				`<Message>setup failed</Message></Error></Errors><RequestID>r</RequestID></Response>`); writeErr != nil {
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
			if _, writeErr := io.WriteString(w, runInstanceXML("i-1")); writeErr != nil {
				panic(writeErr)
			}
		case "DescribeInstances":
			if _, writeErr := io.WriteString(w, describeInstanceXML("i-1", "running")); writeErr != nil {
				panic(writeErr)
			}
		case "AllocateAddress":
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<AllocateAddressResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><publicIp>203.0.113.10</publicIp><domain>vpc</domain>`+
				`<allocationId>eipalloc-1</allocationId></AllocateAddressResponse>`); writeErr != nil {
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
	for _, c := range Addresses().Cases {
		if c.ID == "EC2-EIP-004" {
			spec = c
		}
	}
	got, err := runAddr(context.Background(), rt, spec)
	mu.Lock()
	defer mu.Unlock()
	if err != nil || got.HTTP != http.StatusBadRequest || got.Code != "IncorrectState" || followed {
		t.Fatalf("http %d code %q err %v disassociated %v", got.HTTP, got.Code, err, followed)
	}
}
