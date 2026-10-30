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

func TestJudgeRejectsUndocumentedMessage(t *testing.T) {
	exp := cases.Expect{HTTP: 400, Code: "InvalidVolume.NotFound"}
	bad := cases.Outcome{HTTP: 400, Code: "InvalidVolume.NotFound", Message: "not found"}
	status, reason := Judge(exp, bad)
	if status == "pass" {
		t.Fatalf("undocumented message passed: %s", reason)
	}
	good := cases.Outcome{HTTP: 400, Code: "InvalidVolume.NotFound", Message: "The volume 'vol-abc' does not exist."}
	status, reason = Judge(exp, good)
	if status != "pass" || reason != "" {
		t.Fatalf("documented message status %s reason %q", status, reason)
	}
}

func TestJudgeIAMAccessAndNotFoundMessages(t *testing.T) {
	const (
		accessDenied = "User: arn:aws:iam::123456789012:user/test is not authorized to perform: " +
			"iam:GetMFADevice on resource: arn:aws:iam::123456789012:user/other"
	)
	for name, tc := range map[string]struct {
		exp cases.Expect
		got cases.Outcome
	}{
		"NoSuchEntity": {
			exp: cases.Expect{HTTP: 404, Code: "NoSuchEntity"},
			got: cases.Outcome{
				HTTP: 404, Code: "NoSuchEntity", Message: "The specified MFA device does not exist.",
			},
		},
		"AccessDenied": {
			exp: cases.Expect{HTTP: 403, Code: "AccessDenied"},
			got: cases.Outcome{HTTP: 403, Code: "AccessDenied", Message: accessDenied},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if status, reason := Judge(tc.exp, tc.got); status != "pass" || reason != "" {
				t.Fatalf("status %s reason %q", status, reason)
			}
		})
	}
	for name, tc := range map[string]struct {
		exp cases.Expect
		got cases.Outcome
	}{
		"NoSuchEntity": {
			exp: cases.Expect{HTTP: 404, Code: "NoSuchEntity"},
			got: cases.Outcome{HTTP: 404, Code: "NoSuchEntity", Message: "The specified MFA device is gone."},
		},
		"AccessDenied": {
			exp: cases.Expect{HTTP: 403, Code: "AccessDenied"},
			got: cases.Outcome{HTTP: 403, Code: "AccessDenied", Message: "request rejected"},
		},
	} {
		t.Run(name+" wrong phrase", func(t *testing.T) {
			if status, _ := Judge(tc.exp, tc.got); status != "fail" {
				t.Fatalf("status %s, want fail", status)
			}
		})
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
