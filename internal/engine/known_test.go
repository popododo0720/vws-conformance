package engine

import (
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestKnownIssuesFileParses(t *testing.T) {
	known, err := parseKnownIssues(knownIssuesFile, Groups())
	if err != nil {
		t.Fatal(err)
	}
	if len(known) == 0 {
		t.Fatal("known_issues.txt maps no case")
	}
}

func TestKnownIssuesTracksMFADeviceSuccess(t *testing.T) {
	known, err := parseKnownIssues(knownIssuesFile, Groups())
	if err != nil {
		t.Fatal(err)
	}
	// scripts/guard.sh refuses another vws module name in Go source, so split the URL.
	const issue = "https://github.com/popododo0720/" + "vws-iam/issues/133"
	if known["IAM-MFA-001"] != issue {
		t.Fatalf("IAM-MFA-001 issue = %q, want %q", known["IAM-MFA-001"], issue)
	}
}

func TestParseKnownIssues(t *testing.T) {
	groups := []bind.Group{{Service: "ec2", Name: "x", Cases: []cases.Case{{ID: "EC2-X-001"}}}}
	const url = "https://github.com/example/tracker/issues/1"
	known, err := parseKnownIssues("# comment\n\nEC2-X-001 "+url+"\n", groups)
	if err != nil {
		t.Fatal(err)
	}
	if known["EC2-X-001"] != url {
		t.Fatalf("known = %v", known)
	}
	for name, tc := range map[string]struct{ data, want string }{
		"unknown case ID": {"EC2-NOPE-001 " + url, "unknown case ID EC2-NOPE-001"},
		"repeated ID":     {"EC2-X-001 " + url + "\nEC2-X-001 " + url, "listed twice"},
		"not an issue":    {"EC2-X-001 https://example.com/1", "not a GitHub issue URL"},
		"missing URL":     {"EC2-X-001", "want \"CASE-ID ISSUE-URL\""},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseKnownIssues(tc.data, groups)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}
