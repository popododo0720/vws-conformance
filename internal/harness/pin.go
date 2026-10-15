package harness

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"
)

type Pinned struct {
	URL    string
	ln     net.Listener
	cancel context.CancelFunc
}

func (p *Pinned) Close() error {
	if p == nil {
		return nil
	}
	p.cancel()
	return p.ln.Close()
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
	if err := ResolveEndpointCIDR(ctx, raw, allowed, cidrs); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Pinned{URL: "http://" + ln.Addr().String(), ln: ln, cancel: cancel}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go proxyLatest(ctx, conn, host, port)
		}
	}()
	return p, nil
}

func splitEndpoint(u *url.URL) (string, string, error) {
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", "", fmt.Errorf("endpoint %q is not an http(s) URL", u.String())
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	canon, err := CanonicalHost(host)
	if err != nil {
		return "", "", err
	}
	return canon, port, nil
}

func proxyLatest(ctx context.Context, client net.Conn, host, port string) {
	defer client.Close()
	ips, err := lookupHost(ctx, host)
	if err != nil || len(ips) == 0 {
		return
	}
	up, err := pinDial(ctx, "tcp", net.JoinHostPort(ips[0].String(), port))
	if err != nil {
		return
	}
	defer up.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(up, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, up); done <- struct{}{} }()
	timer := time.NewTimer(OpTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-ctx.Done():
	case <-timer.C:
	}
}

func ChildEnv(base []string) []string {
	return append([]string{}, base...)
}
