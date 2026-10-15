package harness

import (
	"strings"
	"testing"
)

func TestRedactHidesSecret(t *testing.T) {
	secret := "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	token := "session-token-value-123456"
	in := strings.Join([]string{
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261015/vws-1/s3/aws4_request, SignedHeaders=host, Signature=abcdef0123456789",
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
