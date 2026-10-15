package harness

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

var lookupHost = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

func ResolveEndpoint(ctx context.Context, raw string, allowed []string) error {
	return ResolveEndpointCIDR(ctx, raw, allowed, nil)
}

func ResolveEndpointCIDR(ctx context.Context, raw string, allowed, cidrs []string) error {
	host, err := endpointHost(raw)
	if err != nil {
		return err
	}
	if err := CheckEndpointCIDR(raw, allowed, cidrs); err != nil {
		return err
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	_, err = refuseResolved(ctx, host, cidrs)
	return err
}

func DialAllowed(ctx context.Context, network, addr string, allowed, cidrs []string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("endpoint address %q is refused", addr)
	}
	canon, err := CanonicalHost(host)
	if err != nil || canon == "" {
		return nil, fmt.Errorf("endpoint host %q is refused", host)
	}
	if awsHost(canon) {
		return nil, fmt.Errorf("endpoint host %q is refused", canon)
	}
	if ip := net.ParseIP(canon); ip != nil && !addressPermitted(ip, cidrs) {
		return nil, fmt.Errorf("endpoint host %q is refused", canon)
	}
	if !hostListed(canon, allowed) {
		return nil, fmt.Errorf("endpoint host %q is not in the allow list", canon)
	}
	var ips []net.IP
	if net.ParseIP(canon) != nil {
		ips = []net.IP{net.ParseIP(canon)}
	} else if ips, err = refuseResolved(ctx, canon, cidrs); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: OpTimeout}
	var last error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	if last == nil {
		return nil, fmt.Errorf("endpoint host %q has no address", canon)
	}
	return nil, last
}

func refuseResolved(ctx context.Context, host string, cidrs []string) ([]net.IP, error) {
	ips, err := lookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("endpoint host %q did not resolve: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("endpoint host %q did not resolve", host)
	}
	for _, ip := range ips {
		if !addressPermitted(ip, cidrs) {
			return nil, fmt.Errorf("endpoint host %q resolved to %s which is refused", host, ip)
		}
	}
	return ips, nil
}

func NewHTTPClient(allowed, cidrs []string) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return DialAllowed(ctx, network, addr, allowed, cidrs)
	}
	return &http.Client{
		Timeout: OpTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: base,
	}
}
