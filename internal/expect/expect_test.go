package expect

import (
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestJudgeKeepsAWSCode(t *testing.T) {
	exp := cases.Expect{HTTP: 400, Code: "InvalidInstanceID.Malformed"}
	got := cases.Outcome{HTTP: 400, Code: "InvalidParameterValue", Message: "malformed identifier"}
	status, reason := Judge(exp, got)
	if status != "fail" {
		t.Fatalf("status %s (%s)", status, reason)
	}
	if reason != `code got "InvalidParameterValue" want "InvalidInstanceID.Malformed"` {
		t.Fatalf("reason %q", reason)
	}
}

func TestJudgePass(t *testing.T) {
	exp := cases.Expect{HTTP: 200, Patterns: map[string]string{"id": `^i-[0-9a-f]{8,17}$`}}
	got := cases.Outcome{HTTP: 200, Fields: map[string]string{"id": "i-0123456789abcdef0"}}
	status, reason := Judge(exp, got)
	if status != "pass" || reason != "" {
		t.Fatalf("status %s reason %q", status, reason)
	}
}
