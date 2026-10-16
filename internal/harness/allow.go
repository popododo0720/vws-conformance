package harness

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

var awsSuffixes = []string{
	"amazonaws.com",
	"amazonaws.com.cn",
	"amazonaws.eu",
	"api.amazonwebservices.com.cn",
	"api.amazonwebservices.eu",
	"api.aws",
	"api.aws.hci.ic.gov",
	"api.aws.ic.gov",
	"api.aws.scloud",
	"api.cloud-aws.adc-e.uk",
	"aws.dev",
	"awsapps.com",
	"c2s.ic.gov",
	"cloud.adc-e.uk",
	"cloudfront.cn",
	"cloudfront.net",
	"csp.hci.ic.gov",
	"on.aws",
	"sc2s.sgov.gov",
}

func CheckEndpoint(raw string, allowed []string) error {
	return CheckEndpointCIDR(raw, allowed, nil)
}

func CheckEndpointCIDR(raw string, allowed, cidrs []string) error {
	host, err := endpointHost(raw)
	if err != nil {
		return err
	}
	if awsHost(host) {
		return fmt.Errorf("endpoint host %q is refused", host)
	}
	if ip := net.ParseIP(host); ip != nil && !addressPermitted(ip, cidrs) {
		return fmt.Errorf("endpoint host %q is refused", host)
	}
	if !hostListed(host, allowed) {
		return fmt.Errorf("endpoint host %q is not in the allow list", host)
	}
	return nil
}

func endpointHost(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("endpoint %q is not an http(s) URL", raw)
	}
	host, err := CanonicalHost(u.Hostname())
	if err != nil || host == "" {
		return "", fmt.Errorf("endpoint host %q is refused", u.Hostname())
	}
	return host, nil
}

func CanonicalHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	host = strings.TrimRight(host, ".")
	if host == "" {
		return "", fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return canonicalIP(ip).String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil {
		return "", wrapErr("CanonicalHost", err)
	}
	return strings.ToLower(strings.TrimRight(ascii, ".")), nil
}

func awsHost(host string) bool {
	for _, suffix := range awsSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func hostListed(host string, allowed []string) bool {
	for _, a := range allowed {
		c, err := CanonicalHost(a)
		if err != nil {
			continue
		}
		if c == host {
			return true
		}
	}
	return false
}

func canonicalIP(ip net.IP) net.IP {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip
}

func imdsAddress(ip net.IP) bool {
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}
	v6 := net.ParseIP("fd00:ec2::254")
	return v6 != nil && ip.Equal(v6)
}

func addressPermitted(ip net.IP, cidrs []string) bool {
	ip = canonicalIP(ip)
	if ip == nil || imdsAddress(ip) || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if awsPublished(ip) {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4.IsPrivate() {
		return true
	}
	return cidrContains(ip, cidrs)
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
