package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestLoadRefusesAmazonBeforeClientUse(t *testing.T) {
	cfg := Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		EC2Endpoint:     "https://ec2.us-east-1.amazonaws.com",
		AllowedHosts:    []string{"ec2.us-east-1.amazonaws.com"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	}
	_, err := Load(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("got %v", err)
	}
}

func TestRawClientDoesNotFollowRedirect(t *testing.T) {
	hits := 0
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer dest.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusTemporaryRedirect)
	}))
	defer src.Close()
	cl := loadRaw(t, src.URL)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, src.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		panic(closeErr)
	}
	if hits != 0 {
		t.Fatalf("redirect target was dialed %d times", hits)
	}
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestSDKDoesNotFollowRedirect(t *testing.T) {
	hits := 0
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer dest.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusTemporaryRedirect)
	}))
	defer src.Close()
	cl, err := Load(context.Background(), Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		S3Endpoint:      src.URL,
		AllowedHosts:    []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cl.S3.Options().HTTPClient != cl.Raw {
		t.Fatal("sdk http client is not the raw client")
	}
	if _, listErr := cl.S3.ListBuckets(context.Background(), &s3.ListBucketsInput{}); listErr != nil {
		t.Log(listErr)
	}
	if hits != 0 {
		t.Fatalf("redirect target was dialed %d times", hits)
	}
}

func TestRawClientIgnoresProxy(t *testing.T) {
	if os.Getenv("CONF_PROXY_CHILD") == "1" {
		cl := loadRaw(t, "http://127.0.0.1:9")
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://203.0.113.20/blocked", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := cl.Do(req)
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				panic(closeErr)
			}
		}
		if err == nil {
			t.Fatal("public address was dialed")
		}
		return
	}
	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		http.Error(w, "proxy", http.StatusBadGateway)
	}))
	defer proxy.Close()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRawClientIgnoresProxy$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"CONF_PROXY_CHILD=1",
		"HTTP_PROXY="+proxy.URL,
		"http_proxy="+proxy.URL,
		"HTTPS_PROXY="+proxy.URL,
		"https_proxy="+proxy.URL,
		"NO_PROXY=",
		"no_proxy=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if proxyHits != 0 {
		t.Fatalf("proxy was dialed %d times", proxyHits)
	}
}

func TestLoadStaysOnAllowlistedEndpoint(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		raw := `<?xml version="1.0"?>` +
			`<Error><Code>NoSuchBucket</Code><Message>no</Message></Error>`
		if _, writeErr := w.Write([]byte(raw)); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	configPath := dir + "/config"
	if err := os.WriteFile(configPath, []byte("[default]\nendpoint_url = https://s3.us-east-1.amazonaws.com\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AWS_CONFIG_FILE", configPath)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", dir+"/credentials")
	t.Setenv("AWS_ENDPOINT_URL", "https://s3.us-east-1.amazonaws.com")
	t.Setenv("AWS_ENDPOINT_URL_S3", "https://s3.us-east-1.amazonaws.com")
	cl, err := Load(context.Background(), Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		S3Endpoint:      srv.URL,
		AllowedHosts:    []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, listErr := cl.S3.ListBuckets(context.Background(), &s3.ListBucketsInput{}); listErr != nil {
		t.Log(listErr)
	}
	if hits == 0 {
		t.Fatal("request left the allowlisted endpoint")
	}
}

func TestLoadRefusesAWSProfile(t *testing.T) {
	t.Setenv("AWS_PROFILE", "default")
	_, err := Load(context.Background(), Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		S3Endpoint:      "http://127.0.0.1:9",
		AllowedHosts:    []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	})
	if err == nil || !strings.Contains(err.Error(), "AWS_PROFILE") {
		t.Fatalf("%v", err)
	}
}

func loadRaw(t *testing.T, endpoint string) *http.Client {
	t.Helper()
	cl, err := Load(context.Background(), Config{
		AccessKey:       "AKIDEXAMPLE",
		Secret:          "secret",
		Region:          "vws-1",
		S3Endpoint:      endpoint,
		AllowedHosts:    []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	return cl.Raw
}
