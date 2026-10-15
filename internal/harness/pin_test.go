package harness

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
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
	if err == nil && (resp == nil || resp.StatusCode < 500) {
		t.Fatal("changed resolution was proxied")
	}
	for _, addr := range dialed {
		if strings.Contains(addr, "203.0.113.10") {
			t.Fatalf("dialed %s", addr)
		}
	}
}

func TestPinDialsApprovedAddress(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	prevLookup := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() { lookupHost = prevLookup })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := PinEndpoint(ctx, "http://lab.internal:"+port, []string{"lab.internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	resp, err := http.Get(p.URL + "/probe")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || hits != 1 {
		t.Fatalf("status %d hits %d", resp.StatusCode, hits)
	}
}

func TestRealCLIUsesPinnedOrigin(t *testing.T) {
	awsPath, err := exec.LookPath("aws")
	if err != nil {
		awsPath = "/home/ubuntu/.local/bin/aws"
	}
	if _, err := os.Stat(awsPath); err != nil {
		t.Fatal("aws cli is required")
	}
	hits := 0
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult><Owner><ID>x</ID><DisplayName>x</DisplayName></Owner><Buckets></Buckets></ListAllMyBucketsResult>`))
	}))
	defer origin.Close()
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()
	_, port, err := net.SplitHostPort(origin.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	prevLookup := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() { lookupHost = prevLookup })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := PinEndpoint(ctx, "http://lab.internal:"+port, []string{"lab.internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if !strings.HasPrefix(p.URL, "http://127.0.0.1:") {
		t.Fatalf("pin url %s", p.URL)
	}
	cmd := exec.CommandContext(ctx, awsPath, "s3api", "list-buckets", "--endpoint-url", p.URL)
	cmd.Env = append(ChildEnv([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"HTTP_PROXY=" + proxy.URL,
		"HTTPS_PROXY=" + proxy.URL,
		"ALL_PROXY=" + proxy.URL,
		"AWS_PROFILE=default",
	}), "AWS_ACCESS_KEY_ID=AKIDEXAMPLE", "AWS_SECRET_ACCESS_KEY=secret", "AWS_DEFAULT_REGION=vws-1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if hits != 1 || proxyHits != 0 {
		t.Fatalf("origin %d proxy %d", hits, proxyHits)
	}
}

func TestChildEnvDropsContainerCredentials(t *testing.T) {
	env := ChildEnv([]string{
		"PATH=/usr/bin",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI=http://169.254.170.2/creds",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI=/creds",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN=tok",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE=/tmp/tok",
		"AWS_WEB_IDENTITY_TOKEN_FILE=/tmp/web",
		"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/x",
		"AWS_ROLE_SESSION_NAME=conf",
		"AWS_PROFILE=default",
	})
	kept := map[string]bool{"PATH=/usr/bin": false}
	for _, e := range env {
		if _, ok := kept[e]; ok {
			kept[e] = true
		}
		switch {
		case strings.HasPrefix(e, "AWS_CONTAINER_CREDENTIALS_FULL_URI="),
			strings.HasPrefix(e, "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI="),
			strings.HasPrefix(e, "AWS_CONTAINER_AUTHORIZATION_TOKEN="),
			strings.HasPrefix(e, "AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE="),
			strings.HasPrefix(e, "AWS_WEB_IDENTITY_TOKEN_FILE="),
			strings.HasPrefix(e, "AWS_ROLE_ARN="),
			strings.HasPrefix(e, "AWS_ROLE_SESSION_NAME="),
			strings.HasPrefix(e, "AWS_PROFILE="):
			t.Fatalf("kept %s", e)
		}
	}
	if !kept["PATH=/usr/bin"] {
		t.Fatal("PATH was dropped")
	}
}

func TestChildEnvDisablesRemoteParam(t *testing.T) {
	env := ChildEnv([]string{"PATH=/bin", "AWS_CONFIG_FILE=/tmp/real-config"})
	var path string
	for _, e := range env {
		if strings.HasPrefix(e, "AWS_CONFIG_FILE=") {
			path = strings.TrimPrefix(e, "AWS_CONFIG_FILE=")
		}
	}
	if path == "" || path == "/tmp/real-config" {
		t.Fatalf("config file %q", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "cli_follow_urlparam = false") {
		t.Fatalf("config %q is %q", path, b)
	}
}

func TestPinKeepsSignedHost(t *testing.T) {
	const access = "AKIDEXAMPLE"
	const secret = "secret"
	const region = "vws-1"
	var gotHost, gotAuth, gotDate, gotHash, gotPath string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotAuth = r.Header.Get("Authorization")
		gotDate = r.Header.Get("X-Amz-Date")
		gotHash = r.Header.Get("X-Amz-Content-Sha256")
		gotPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()
	_, port, err := net.SplitHostPort(origin.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	prevLookup := lookupHost
	lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	t.Cleanup(func() { lookupHost = prevLookup })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p, err := PinEndpoint(ctx, "http://lab.internal:"+port, []string{"lab.internal"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL+"/probe", nil)
	if err != nil {
		t.Fatal(err)
	}
	creds := aws.Credentials{AccessKeyID: access, SecretAccessKey: secret}
	err = v4.NewSigner().SignHTTP(ctx, creds, req, PayloadHash(nil), "s3", region, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || gotAuth == "" || gotDate == "" {
		t.Fatalf("status %d auth %q date %q", resp.StatusCode, gotAuth, gotDate)
	}
	when, err := time.Parse("20060102T150405Z", gotDate)
	if err != nil {
		t.Fatal(err)
	}
	again, err := http.NewRequest(http.MethodGet, "http://"+gotHost+gotPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	hash := gotHash
	if hash == "" {
		hash = PayloadHash(nil)
	}
	err = v4.NewSigner().SignHTTP(ctx, creds, again, hash, "s3", region, when)
	if err != nil {
		t.Fatal(err)
	}
	if again.Header.Get("Authorization") != gotAuth {
		t.Fatalf("host %q path %q\norigin %s\nagain  %s",
			gotHost, gotPath, gotAuth, again.Header.Get("Authorization"))
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
