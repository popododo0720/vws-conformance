package harness

import (
	"net/http"
	"regexp"
	"strings"
)

var (
	reAuth      = regexp.MustCompile(`(?i)(Authorization:\s*).*`)
	reSig       = regexp.MustCompile(`(?i)(X-Amz-Signature=)([0-9a-fA-F]+)`)
	reToken     = regexp.MustCompile(`(?i)(X-Amz-Security-Token[=:]\s*)(\S+)`)
	reCred      = regexp.MustCompile(`(?i)(X-Amz-Credential=)([^/\s]+)`)
	reSecretKey = regexp.MustCompile(`(SecretAccessKey|SessionToken|KeyMaterial|Password)(["=:]\s*)([^"&\s<]+)`)
	reXMLSecret = regexp.MustCompile(`(?i)(<(?:SecretAccessKey|SessionToken|KeyMaterial|Password)>)([^<]*)(</)`)
	// S3 SignatureDoesNotMatch bodies echo the access key ID, the signature and
	// the canonical request, whose hex form can carry a signed session token.
	reXMLKeyID = regexp.MustCompile(`(?i)(<AWSAccessKeyId>)([^<]*)(</)`)
	reXMLSign  = regexp.MustCompile(`(?i)(<(?:SignatureProvided|CanonicalRequestBytes)>)([^<]*)(</)`)
)

func Redact(text string, secrets ...string) string {
	text = reAuth.ReplaceAllString(text, "${1}REDACTED")
	text = reSig.ReplaceAllString(text, "${1}REDACTED")
	text = reToken.ReplaceAllString(text, "${1}REDACTED")
	text = reCred.ReplaceAllStringFunc(text, func(m string) string {
		parts := reCred.FindStringSubmatch(m)
		if len(parts) < 3 {
			return "X-Amz-Credential=REDACTED"
		}
		return parts[1] + keyPrefix(parts[2]) + "REDACTED"
	})
	text = reSecretKey.ReplaceAllString(text, "${1}${2}REDACTED")
	text = reXMLSecret.ReplaceAllString(text, "${1}REDACTED${3}")
	text = reXMLKeyID.ReplaceAllStringFunc(text, func(m string) string {
		parts := reXMLKeyID.FindStringSubmatch(m)
		return parts[1] + keyPrefix(parts[2]) + "REDACTED" + parts[3]
	})
	text = reXMLSign.ReplaceAllString(text, "${1}REDACTED${3}")
	for _, s := range secrets {
		if s == "" || len(s) < 6 {
			continue
		}
		text = strings.ReplaceAll(text, s, "REDACTED")
	}
	return text
}

// keyPrefix keeps the 4-character access key type prefix (AKIA, ASIA) for debugging.
func keyPrefix(id string) string {
	if len(id) > 4 {
		return id[:4]
	}
	return id
}

func RedactHeader(h http.Header) http.Header {
	out := h.Clone()
	if out == nil {
		return http.Header{}
	}
	for _, k := range []string{"Authorization", "X-Amz-Security-Token"} {
		if out.Get(k) != "" {
			out.Set(k, "REDACTED")
		}
	}
	return out
}
