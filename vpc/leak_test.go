package vpc

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

func TestUnexpectedVpcCreateIsTracked(t *testing.T) {
	var mu sync.Mutex
	deleted := false
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
		if action == "DeleteVpc" {
			mu.Lock()
			deleted = true
			mu.Unlock()
			if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
				`<DeleteVpcResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
				`<requestId>r</requestId><return>true</return></DeleteVpcResponse>`); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if action != "CreateVpc" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<CreateVpcResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">`+
			`<requestId>r</requestId><vpc><vpcId>vpc-leak</vpcId><state>available</state>`+
			`<cidrBlock>10.0.0.0/33</cidrBlock></vpc></CreateVpcResponse>`); writeErr != nil {
			panic(writeErr)
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
	for _, c := range Vpcs().Cases {
		if c.ID == "VPC-VPC-101" {
			spec = c
		}
	}
	if _, err := runVpcs(context.Background(), rt, spec); err != nil {
		t.Fatal(err)
	}
	if err := rt.Clean.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !deleted {
		t.Fatal("successful create left vpc-leak")
	}
}
