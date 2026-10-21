package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestPlantedIDsStayUntouchedByNegativeCalls(t *testing.T) {
	const (
		plantedInstance = "i-0123456789abcdef0"
		plantedImage    = "ami-0123456789abcdef1"
		plantedAssoc    = "eipassoc-0123456789abcdef0"
		run             = "conf-20261016T120000Z-aabbcc"
	)
	mutating := map[string]bool{
		"StopInstances": true, "StartInstances": true, "TerminateInstances": true,
		"RebootInstances": true, "ModifyInstanceAttribute": true, "CreateImage": true,
		"DeleteTags": true, "DeregisterImage": true, "DisassociateAddress": true,
	}
	var mu sync.Mutex
	var hits []string
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		vals, queryErr := url.ParseQuery(string(b))
		if queryErr != nil {
			panic(queryErr)
		}
		action := vals.Get("Action")
		mu.Lock()
		seen = append(seen, action)
		if mutating[action] && (strings.Contains(vals.Encode(), plantedInstance) ||
			strings.Contains(vals.Encode(), plantedImage) || strings.Contains(vals.Encode(), plantedAssoc)) {
			hits = append(hits, action+" "+vals.Encode())
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/xml")
		if _, writeErr := io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><unused/>`); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: run, OutDir: t.TempDir(), AccountID: "123456789012",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	type call struct {
		id  string
		run func(context.Context, *harness.Runtime, cases.Case) (cases.Outcome, error)
		set []cases.Case
	}
	calls := []call{
		{"EC2-STOP-101", runInstances, Instances().Cases},
		{"EC2-START-101", runInstances, Instances().Cases},
		{"EC2-TERM-101", runInstances, Instances().Cases},
		{"EC2-GAP-001", runEC2Gaps, Gaps().Cases},
		{"EC2-GAP-002", runEC2Gaps, Gaps().Cases},
		{"EC2-GAP-005", runEC2Gaps, Gaps().Cases},
		{"EC2-GAP-006", runEC2Gaps, Gaps().Cases},
		{"EC2-GAP-007", runEC2Gaps, Gaps().Cases},
		{"EC2-EIP-102", runAddr, Addresses().Cases},
	}
	for _, c := range calls {
		spec := findCase(t, c.set, c.id)
		if _, callErr := c.run(context.Background(), rt, spec); callErr == nil ||
			!strings.Contains(callErr.Error(), "vws-conformance#1") {
			t.Errorf("%s err %v", c.id, callErr)
		}
		if !strings.Contains(spec.Skip, "vws-conformance#1") {
			t.Errorf("%s skip %q", c.id, spec.Skip)
		}
	}
	if _, callErr := runEC2Gaps(context.Background(), rt, findCase(t, Gaps().Cases, "EC2-GAP-003")); callErr != nil {
		t.Fatal(callErr)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hits) > 0 {
		t.Fatalf("mutated planted ids: %v", hits)
	}
	if !containsAction(seen, "DescribeInstanceStatus") {
		t.Fatalf("control describe was not sent, seen %v", seen)
	}
}

func findCase(t *testing.T, list []cases.Case, id string) cases.Case {
	t.Helper()
	for _, c := range list {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing case %s", id)
	return cases.Case{}
}

func containsAction(seen []string, action string) bool {
	for _, item := range seen {
		if item == action {
			return true
		}
	}
	return false
}
