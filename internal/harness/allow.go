package harness

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func CheckEndpoint(raw string, allowed []string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("endpoint %q is not an http(s) URL", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("endpoint %q has no host", raw)
	}
	if host == "amazonaws.com" || strings.HasSuffix(host, ".amazonaws.com") {
		return fmt.Errorf("endpoint host %q is refused", host)
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() {
		return fmt.Errorf("endpoint host %q is not a private address", host)
	}
	ok := false
	for _, a := range allowed {
		if strings.EqualFold(a, host) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("endpoint host %q is not in the allow list", host)
	}
	return nil
}

func CheckAccount(account string, allowed []string) error {
	if len(account) != 12 {
		return fmt.Errorf("account %q is not 12 digits", account)
	}
	for _, c := range account {
		if c < '0' || c > '9' {
			return fmt.Errorf("account %q is not 12 digits", account)
		}
	}
	for _, a := range allowed {
		if a == account {
			return nil
		}
	}
	return fmt.Errorf("account %s is not allowed", account)
}
