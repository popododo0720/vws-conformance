package harness

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type Pinned struct {
	URL    string
	srv    *http.Server
	cancel context.CancelFunc
}

func (p *Pinned) Close() error {
	if p == nil {
		return nil
	}
	p.cancel()
	return p.srv.Close()
}

var pinDial = func(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{Timeout: OpTimeout}).DialContext(ctx, network, addr)
}

func PinEndpoint(ctx context.Context, raw string, allowed, cidrs []string) (*Pinned, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	host, port, err := splitEndpoint(u)
	if err != nil {
		return nil, err
	}
	if err := CheckEndpointCIDR(raw, allowed, cidrs); err != nil {
		return nil, err
	}
	pinned, err := approvedIPs(ctx, host, cidrs)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			now, err := approvedIPs(ctx, host, cidrs)
			if err != nil {
				return nil, err
			}
			if !sameIPs(now, pinned) {
				return nil, fmt.Errorf("endpoint host %q resolution changed", host)
			}
			return pinDial(ctx, network, net.JoinHostPort(canonicalIP(pinned[0]).String(), port))
		},
	}
	if u.Scheme == "https" {
		transport.TLSClientConfig = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(&url.URL{Scheme: u.Scheme, Host: net.JoinHostPort(host, port)})
			pr.Out.Host = u.Host
		},
		Transport: transport,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, err
	}
	srv := &http.Server{Handler: proxy}
	go func() { _ = srv.Serve(ln) }()
	return &Pinned{URL: "http://" + ln.Addr().String(), srv: srv, cancel: cancel}, nil
}

func splitEndpoint(u *url.URL) (string, string, error) {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", fmt.Errorf("endpoint %q is not an http(s) URL", u.String())
	}
	host, err := CanonicalHost(u.Hostname())
	if err != nil || host == "" {
		return "", "", fmt.Errorf("endpoint host %q is refused", u.Hostname())
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return host, port, nil
}

func approvedIPs(ctx context.Context, host string, cidrs []string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !addressPermitted(ip, cidrs) {
			return nil, fmt.Errorf("endpoint host %q is refused", host)
		}
		return []net.IP{canonicalIP(ip)}, nil
	}
	return refuseResolved(ctx, host, cidrs)
}

func sameIPs(a, b []net.IP) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, ip := range a {
		seen[canonicalIP(ip).String()]++
	}
	for _, ip := range b {
		k := canonicalIP(ip).String()
		seen[k]--
		if seen[k] < 0 {
			return false
		}
	}
	return true
}

func ChildEnv(base []string) []string {
	drop := map[string]bool{
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "ALL_PROXY": true, "NO_PROXY": true,
		"http_proxy": true, "https_proxy": true, "all_proxy": true, "no_proxy": true,
		"AWS_PROFILE": true, "AWS_DEFAULT_PROFILE": true,
		"AWS_ENDPOINT_URL": true, "AWS_ENDPOINT_URL_S3": true,
		"AWS_SESSION_TOKEN": true, "AWS_SECURITY_TOKEN": true,
		"AWS_ACCESS_KEY_ID": true, "AWS_SECRET_ACCESS_KEY": true,
		"AWS_SHARED_CREDENTIALS_FILE": true, "AWS_CONFIG_FILE": true,
	}
	var out []string
	for _, e := range base {
		k, _, ok := strings.Cut(e, "=")
		if ok && drop[k] {
			continue
		}
		out = append(out, e)
	}
	return append(out,
		"AWS_SHARED_CREDENTIALS_FILE=/dev/null",
		"AWS_CONFIG_FILE=/dev/null",
		"AWS_EC2_METADATA_DISABLED=true",
	)
}
