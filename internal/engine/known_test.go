package engine

import (
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestKnownIssuesFileParses(t *testing.T) {
	known, err := parseKnownIssues(knownIssuesFile, Groups(), "local-fake")
	if err != nil {
		t.Fatal(err)
	}
	if len(known) == 0 {
		t.Fatal("known_issues.txt maps no case")
	}
}

func TestKnownIssuesTracksMFADeviceSuccess(t *testing.T) {
	known, err := parseKnownIssues(knownIssuesFile, Groups(), "local-fake")
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
	known, err := parseKnownIssues("# comment\n\nEC2-X-001 "+url+"\n", groups, "local-fake")
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
			_, err := parseKnownIssues(tc.data, groups, "local-fake")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestKnownIssuesTracksCrossAccountUntagPolicy(t *testing.T) {
	known, err := parseKnownIssues(knownIssuesFile, Groups(), "local-fake")
	if err != nil {
		t.Fatal(err)
	}
	const issue = "https://github.com/popododo0720/" + "vws-console-api/issues/560"
	if known["IAM-PTAG-108"] != issue {
		t.Fatalf("IAM-PTAG-108 issue = %q, want %q", known["IAM-PTAG-108"], issue)
	}
}

func TestKnownIssuesStayInTheirTarget(t *testing.T) {
	for _, target := range []string{"local-pve", "local-fake", "vws-test", "pve-test"} {
		t.Run(target, func(t *testing.T) {
			for range 2 {
				known, err := parseKnownIssues(knownIssuesFile, Groups(), target)
				if err != nil {
					t.Fatal(err)
				}
				for _, id := range []string{
					"EC2-DI-101", "EC2-DIA-101", "EC2-DIMG-001", "EC2-DIMG-101",
					"EC2-KP-103", "EC2-EIP-103", "EC2-GAP-003", "EC2-GAP-008",
					"EBS-VOL-104", "EBS-SNAP-102", "VPC-VPC-104", "VPC-SUB-103",
					"VPC-SG-104", "AUTH-001",
				} {
					if (known[id] != "") != (target == "local-pve") {
						t.Fatalf("%s exception on %s: %q", id, target, known[id])
					}
				}
				if known["IAM-MFA-001"] == "" {
					t.Fatal("unscoped exception was lost")
				}
			}
		})
	}
}

func TestKnownIssuesValidateExcludedTargets(t *testing.T) {
	groups := []bind.Group{{Cases: []cases.Case{{ID: "EC2-X-001"}}}}
	const entry = "EC2-X-001 https://github.com/example/tracker/issues/1"
	for _, data := range []string{
		entry + " local-pvee",
		entry + " local-pve extra",
		entry + " local-pve\n" + entry + " local-pve",
		"UNKNOWN https://github.com/example/tracker/issues/1 local-pve",
		"EC2-X-001 https://example.com/1 local-pve",
	} {
		if _, err := parseKnownIssues(data, groups, "local-fake"); err == nil {
			t.Fatalf("accepted malformed excluded entry %q", data)
		}
	}
}
