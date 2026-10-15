package harness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPinRefusesChangedResolution(t *testing.T) {
	calls := 0
	prevLookup := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		calls++
		if calls == 1 {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	t.Cleanup(func() { lookupHost = prevLookup })
	var dialed []string
	prevDial := pinDial
	pinDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialed = append(dialed, addr)
		return nil, fmt.Errorf("dial %s", addr)
	}
	t.Cleanup(func() { pinDial = prevDial })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := PinEndpoint(ctx, "http://lab.internal:8082", []string{"lab.internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/probe", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	_ = err
	for _, addr := range dialed {
		if strings.Contains(addr, "203.0.113.10") {
			t.Fatalf("dialed %s", addr)
		}
	}
	if len(dialed) == 0 {
		t.Fatal("pinned address was not dialed")
	}
}

func TestChildEnvDropsProxy(t *testing.T) {
	env := ChildEnv([]string{
		"PATH=/usr/bin",
		"HTTP_PROXY=http://127.0.0.1:9",
		"HTTPS_PROXY=http://127.0.0.1:9",
		"ALL_PROXY=http://127.0.0.1:9",
		"AWS_PROFILE=default",
		"AWS_ENDPOINT_URL=https://s3.us-east-1.amazonaws.com",
	})
	for _, e := range env {
		switch {
		case strings.HasPrefix(e, "HTTP_PROXY="), strings.HasPrefix(e, "HTTPS_PROXY="), strings.HasPrefix(e, "ALL_PROXY="), strings.HasPrefix(e, "AWS_PROFILE="), strings.HasPrefix(e, "AWS_ENDPOINT_URL="):
			t.Fatalf("kept %s", e)
		}
	}
}
