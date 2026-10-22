package harness

import (
	"fmt"
	"strings"
	"testing"
)

func TestRedactHidesSecret(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	token := "session-token-value-123456"
	in := strings.Join([]string{
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261015/vws-1/s3/aws4_request, " +
			"SignedHeaders=host, Signature=abcdef0123456789",
		"X-Amz-Security-Token: " + token,
		"X-Amz-Credential=AKIDEXAMPLE/20261015/vws-1/s3/aws4_request",
		"X-Amz-Signature=deadbeef",
		"<SecretAccessKey>" + secret + "</SecretAccessKey>",
		"<keyMaterial>" + secret + "</keyMaterial>",
		"SessionToken=" + token,
		"KeyMaterial=" + secret,
		"Password=" + secret,
	}, "\n")
	out := Redact(in, secret, token)
	for _, s := range []string{secret, token, "abcdef0123456789", "deadbeef", "AKIDEXAMPLE/20261015"} {
		if strings.Contains(out, s) {
			t.Fatalf("leaked %q in %s", s, out)
		}
	}
	if !strings.Contains(out, "AKIDREDACTED") {
		t.Fatalf("key prefix missing: %s", out)
	}
}

// TestRedactSignatureDoesNotMatchBody checks the S3 SignatureDoesNotMatch
// error body, which echoes the caller's access key ID, the signature and the
// canonical request (with any signed session token) back to the client.
func TestRedactSignatureDoesNotMatchBody(t *testing.T) {
	const (
		key   = "AKIDEXAMPLECONFTEST01"
		token = "session-token-value-123456"
		sig   = "f3a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff"
	)
	tokenHex := hexBytes("x-amz-security-token:" + token)
	in := "<Error><Code>SignatureDoesNotMatch</Code>" +
		"<Message>The request signature we calculated does not match the signature you provided.</Message>" +
		"<AWSAccessKeyId>" + key + "</AWSAccessKeyId>" +
		"<StringToSign>AWS4-HMAC-SHA256\n20261015T000000Z\n20261015/vws-1/s3/aws4_request\nabc</StringToSign>" +
		"<SignatureProvided>" + sig + "</SignatureProvided>" +
		"<CanonicalRequest>GET\n/\n\nhost:vws\nx-amz-security-token:" + token + "\n</CanonicalRequest>" +
		"<CanonicalRequestBytes>" + tokenHex + "</CanonicalRequestBytes>" +
		"<RequestId>1</RequestId></Error>"
	out := Redact(in)
	for _, s := range []string{key, token, sig, tokenHex} {
		if strings.Contains(out, s) {
			t.Fatalf("leaked %q in %s", s, out)
		}
	}
	for _, s := range []string{"<AWSAccessKeyId>AKIDREDACTED</AWSAccessKeyId>", "<Code>SignatureDoesNotMatch</Code>"} {
		if !strings.Contains(out, s) {
			t.Fatalf("%q missing in %s", s, out)
		}
	}
}

func hexBytes(s string) string {
	parts := make([]string, 0, len(s))
	for i := 0; i < len(s); i++ {
		parts = append(parts, fmt.Sprintf("%02x", s[i]))
	}
	return strings.Join(parts, " ")
}
