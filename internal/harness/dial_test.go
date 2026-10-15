package harness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDialConnectsToResolvedIP(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	prev := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		if host != "lab.internal" {
			return nil, fmt.Errorf("lookup %s", host)
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() { lookupHost = prev })
	cl := NewHTTPClient([]string{"lab.internal"}, nil)
	resp, err := cl.Get("http://lab.internal:" + port + "/x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hits != 1 || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("hits %d status %d", hits, resp.StatusCode)
	}
}

func TestDialRefusesMixedAnswers(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	prev := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10"), net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() { lookupHost = prev })
	cl := NewHTTPClient([]string{"lab.internal"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://lab.internal:"+port+"/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cl.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || hits != 0 {
		t.Fatalf("err %v hits %d", err, hits)
	}
}

func TestResolveRefusesPublicAnswer(t *testing.T) {
	prev := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("203.0.113.10")}, nil
	}
	t.Cleanup(func() { lookupHost = prev })
	err := ResolveEndpoint(context.Background(), "http://lab.internal:8082", []string{"lab.internal"})
	if err == nil {
		t.Fatal("accepted a public answer")
	}
}
