package harness

import (
	"bufio"
	"embed"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
)

//go:embed aws-ranges.txt
var awsRangesFile embed.FS

var (
	awsRangesOnce sync.Once
	awsRanges     []netip.Prefix
)

func publishedRanges() []netip.Prefix {
	awsRangesOnce.Do(func() {
		b, err := awsRangesFile.ReadFile("aws-ranges.txt")
		if err != nil {
			return
		}
		sc := bufio.NewScanner(strings.NewReader(string(b)))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			p, err := netip.ParsePrefix(line)
			if err != nil {
				continue
			}
			awsRanges = append(awsRanges, p.Masked())
		}
	})
	return awsRanges
}

func addrFrom(ip net.IP) (netip.Addr, bool) {
	ip = canonicalIP(ip)
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

func awsPublished(ip net.IP) bool {
	addr, ok := addrFrom(ip)
	if !ok {
		return true
	}
	for _, p := range publishedRanges() {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func privateSupernets() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "100.64.0.0/10", "fc00::/7",
		"::1/128",
	} {
		p, err := netip.ParsePrefix(s)
		if err == nil {
			out = append(out, p)
		}
	}
	return out
}

func privatePrefix(p netip.Prefix) bool {
	p = p.Masked()
	for _, super := range privateSupernets() {
		if super.Addr().BitLen() == p.Addr().BitLen() && super.Contains(p.Addr()) && p.Bits() >= super.Bits() {
			return true
		}
	}
	return false
}

func prefixHitsAWS(p netip.Prefix) bool {
	p = p.Masked()
	for _, a := range publishedRanges() {
		if a.Addr().BitLen() == p.Addr().BitLen() && a.Overlaps(p) {
			return true
		}
	}
	return false
}

func validateCIDRs(cidrs []string) error {
	for _, raw := range cidrs {
		p, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || !privatePrefix(p) || prefixHitsAWS(p) {
			return fmt.Errorf("VWS_CONF_ALLOWED_CIDRS entry %q is refused", raw)
		}
	}
	return nil
}

func cidrContains(ip net.IP, cidrs []string) bool {
	addr, ok := addrFrom(ip)
	if !ok {
		return false
	}
	for _, raw := range cidrs {
		p, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || !privatePrefix(p) || prefixHitsAWS(p) {
			continue
		}
		if p.Contains(addr) {
			return true
		}
	}
	return false
}
