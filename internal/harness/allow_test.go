package harness

import (
	"strings"
	"testing"
)

func TestRejectsAmazonAWSEvenIfListed(t *testing.T) {
	err := CheckEndpoint("https://s3.us-east-1.amazonaws.com", []string{"s3.us-east-1.amazonaws.com"})
	if err == nil {
		t.Fatal("expected reject")
	}
	if err.Error() != `endpoint host "s3.us-east-1.amazonaws.com" is refused` {
		t.Fatalf("%v", err)
	}
	err = CheckEndpoint("https://amazonaws.com/bucket", []string{"amazonaws.com"})
	if err == nil || err.Error() != `endpoint host "amazonaws.com" is refused` {
		t.Fatalf("%v", err)
	}
}

func TestRejectsDisguisedAWSHosts(t *testing.T) {
	cases := []struct{ raw, host string }{
		{"https://s3.us-east-1.amazonaws.com.", "s3.us-east-1.amazonaws.com."},
		{"https://S3.US-EAST-1.AMAZONAWS.COM", "S3.US-EAST-1.AMAZONAWS.COM"},
		{"https://s3.cn-north-1.amazonaws.com.cn", "s3.cn-north-1.amazonaws.com.cn"},
		{"https://bedrock.us-east-1.api.aws", "bedrock.us-east-1.api.aws"},
		{"https://foo.aws.dev", "foo.aws.dev"},
		{"https://bar.on.aws", "bar.on.aws"},
		{"https://d111111abcdef8.cloudfront.net", "d111111abcdef8.cloudfront.net"},
		{"https://s3.eu-central-1.amazonaws.eu", "s3.eu-central-1.amazonaws.eu"},
		{"https://console.awsapps.com", "console.awsapps.com"},
		{"https://s3.us-iso-east-1.c2s.ic.gov", "s3.us-iso-east-1.c2s.ic.gov"},
		{"https://s3.us-isob-east-1.sc2s.sgov.gov", "s3.us-isob-east-1.sc2s.sgov.gov"},
		{"https://s3.cn-north-1.api.amazonwebservices.com.cn", "s3.cn-north-1.api.amazonwebservices.com.cn"},
		{"https://ec2.eu-central-1.api.amazonwebservices.eu", "ec2.eu-central-1.api.amazonwebservices.eu"},
		{"https://s3.eu-isoe-west-1.cloud.adc-e.uk", "s3.eu-isoe-west-1.cloud.adc-e.uk"},
		{"https://s3.us-isof-south-1.csp.hci.ic.gov", "s3.us-isof-south-1.csp.hci.ic.gov"},
		{"https://d111111abcdef8.cloudfront.cn", "d111111abcdef8.cloudfront.cn"},
		{"https://s3.us-east-1." + fw("amazonaws") + ".com", "s3.us-east-1." + fw("amazonaws") + ".com"},
	}
	for _, tc := range cases {
		err := CheckEndpoint(tc.raw, []string{tc.host})
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("%s: %v", tc.raw, err)
		}
	}
}

func fw(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r + 0xFEE0)
	}
	return b.String()
}

func TestRejectsLinkLocalEvenIfListed(t *testing.T) {
	for _, raw := range []string{
		"http://169.254.169.254/",
		"http://169.254.169.254./",
		"http://[fe80::1]/",
		"http://0.0.0.0/",
		"http://224.0.0.1/",
	} {
		host := raw[len("http://") : len(raw)-1]
		err := CheckEndpoint(raw, []string{host, "169.254.169.254", "fe80::1", "0.0.0.0", "224.0.0.1"})
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestListedPublicIPIsRefused(t *testing.T) {
	err := CheckEndpoint("http://203.0.113.10:8082", []string{"203.0.113.10"})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("%v", err)
	}
}

func TestListedAWSAddressIsRefused(t *testing.T) {
	for _, raw := range []string{"http://3.4.12.4/", "http://[2406:daba:f000::1]/"} {
		host := "3.4.12.4"
		if strings.Contains(raw, "2406") {
			host = "2406:daba:f000::1"
		}
		err := CheckEndpoint(raw, []string{host})
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
}

func TestConfiguredPrivateCIDR(t *testing.T) {
	err := CheckEndpoint("http://100.64.1.5:8082", []string{"100.64.1.5"})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("cgnat without cidr: %v", err)
	}
	if err := CheckEndpointCIDR("http://100.64.1.5:8082", []string{"100.64.1.5"}, []string{"100.64.0.0/10"}); err != nil {
		t.Fatal(err)
	}
	err = CheckEndpointCIDR("http://3.4.12.4/", []string{"3.4.12.4"}, []string{"3.4.12.4/32"})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("aws prefix inside a cidr: %v", err)
	}
	if err := CheckEndpoint("http://10.77.0.241:8081", []string{"10.77.0.241"}); err != nil {
		t.Fatal(err)
	}
}

func TestIMDSAddressesAreRefused(t *testing.T) {
	rows := []struct {
		raw  string
		host string
		cidr string
	}{
		{raw: "http://169.254.169.254/", host: "169.254.169.254", cidr: "169.254.0.0/16"},
		{raw: "http://[fd00:ec2::254]/", host: "fd00:ec2::254", cidr: "fc00::/7"},
	}
	for _, row := range rows {
		err := CheckEndpointCIDR(row.raw, []string{row.host}, []string{row.cidr})
		if err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("%s: %v", row.raw, err)
		}
	}
}

func TestAllowLoopback(t *testing.T) {
	if err := CheckEndpoint("http://127.0.0.1:8082", []string{"127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnlistedHost(t *testing.T) {
	err := CheckEndpoint("http://127.0.0.1:8082", []string{"10.77.0.241"})
	if err == nil || err.Error() != `endpoint host "127.0.0.1" is not in the allow list` {
		t.Fatalf("%v", err)
	}
}

func TestAccountGuard(t *testing.T) {
	if err := CheckAccount("123456789012", []string{"123456789012"}); err != nil {
		t.Fatal(err)
	}
	err := CheckAccount("999999999999", []string{"123456789012"})
	if err == nil || err.Error() != "account 999999999999 is not allowed" {
		t.Fatalf("%v", err)
	}
}
