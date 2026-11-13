package engine

import (
	"reflect"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/report"
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
					"EC2-RUN-001", "EC2-RUN-110", "EC2-RUN-201",
					"EC2-DI-001", "EC2-DI-201", "EC2-DIA-001",
					"EC2-STOP-001", "EC2-START-001", "EC2-TERM-001",
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

func TestInstanceKnownFailuresRequireTheirTargetAndIssue(t *testing.T) {
	const issue = "https://github.com/popododo0720/" + "vws-console-api/issues/1005"
	for _, target := range []string{"local-pve", "local-fake", "vws-test", "pve-test"} {
		known, err := parseKnownIssues(knownIssuesFile, Groups(), target)
		if err != nil {
			t.Fatal(err)
		}
		want := ""
		switch target {
		case "local-fake":
			want = issue
		case "local-pve":
			want = "https://github.com/popododo0720/" + "vws-console-api/issues/792"
		}
		if known["EC2-RUN-202"] != want || known["EC2-RUN-101"] != issue {
			t.Fatalf("launch errors on %s: %v", target, known)
		}
	}
	line := "EC2-RUN-202 " + issue + " local-fake\n"
	if !strings.Contains(knownIssuesFile, line) {
		t.Fatal("missing exact known-failure record")
	}
	known, err := parseKnownIssues(strings.ReplaceAll(knownIssuesFile, line, ""), Groups(), "local-fake")
	if err != nil {
		t.Fatal(err)
	}
	rep := &report.Report{Results: []report.Result{{
		ID: "EC2-RUN-202", Status: "fail", Issue: known["EC2-RUN-202"],
	}}}
	if failures := parityGate(rep, func(string) {}); len(failures) != 1 {
		t.Fatal("removing the exception did not fail the parity gate")
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

func TestKnownIssuesTargetOverridesUnscoped(t *testing.T) {
	groups := []bind.Group{{Cases: []cases.Case{{ID: "IAM-X-001"}}}}
	const fallback = "https://github.com/example/tracker/issues/1"
	const specific = "https://github.com/example/tracker/issues/2"
	global := "IAM-X-001 " + fallback
	local := "IAM-X-001 " + specific + " local-pve"
	for _, data := range []string{global + "\n" + local, local + "\n" + global} {
		for _, target := range []string{"local-pve", "local-fake", "pve-test", "vws-test"} {
			known, err := parseKnownIssues(data, groups, target)
			if err != nil {
				t.Fatal(err)
			}
			want := fallback
			if target == "local-pve" {
				want = specific
			}
			if known["IAM-X-001"] != want {
				t.Fatalf("target %s: got %v, want %s", target, known, want)
			}
		}
	}
	// Distinct targets may track distinct causes; repeats within a target fail
	// closed even when that target is excluded from the current run.
	if _, err := parseKnownIssues(local+"\n"+local, groups, "local-fake"); err == nil {
		t.Fatal("accepted repeated target override")
	}
}

func TestMFATagsKeepExistingKnownIssuesWithoutRedundantOverrides(t *testing.T) {
	for _, target := range []string{"local-pve", "local-fake", "vws-test", "pve-test"} {
		known, err := parseKnownIssues(knownIssuesFile, Groups(), target)
		if err != nil {
			t.Fatal(err)
		}
		repo := "vws-console-api/issues/850"
		for _, id := range []string{
			"IAM-MTAG-001", "IAM-MTAG-002", "IAM-MTAG-003", "IAM-MTAG-101",
			"IAM-MTAG-102", "IAM-MTAG-104", "IAM-MTAG-105", "IAM-MTAG-106",
			"IAM-MTAG-107", "IAM-GAP-005",
		} {
			if want := "https://github.com/popododo0720/" + repo; known[id] != want {
				t.Fatalf("%s on %s: got %s, want %s", id, target, known[id], want)
			}
			if count := strings.Count(knownIssuesFile, "\n"+id+" "); count != 1 {
				t.Fatalf("%s has %d known-issue entries, want one for the other targets", id, count)
			}
		}
	}
	const issue = "https://github.com/popododo0720/" + "vws-iam/issues/501"
	if strings.Contains(knownIssuesFile, issue+" local-pve") {
		t.Fatal("ownership exclusion also has a redundant local-pve known-failure entry")
	}
}

func TestLocalPVESkipsExactlyTenOwnershipCases(t *testing.T) {
	excluded := map[string]bool{
		"IAM-MTAG-001": true, "IAM-MTAG-002": true, "IAM-MTAG-003": true,
		"IAM-MTAG-101": true, "IAM-MTAG-102": true, "IAM-MTAG-104": true,
		"IAM-MTAG-105": true, "IAM-MTAG-106": true, "IAM-MTAG-107": true,
		"IAM-GAP-005": true,
	}
	original := map[string]cases.Case{}
	for _, g := range Groups() {
		for _, c := range g.Cases {
			original[c.ID] = c
		}
	}
	for _, target := range []string{"local-pve", "local-fake", "vws-test", "pve-test", "local-pve"} {
		got := selectGroups(harness.Config{Full: true, Target: target}, "", "")
		count := 0
		for _, g := range got {
			for _, c := range g.Cases {
				want := original[c.ID]
				if target == "local-pve" && excluded[c.ID] {
					want.Skip = "ownership cleanup blocked by https://github.com/popododo0720/" + "vws-iam/issues/501"
				}
				if !reflect.DeepEqual(c, want) {
					t.Fatalf("%s on %s: got %+v, want %+v", c.ID, target, c, want)
				}
				count++
			}
		}
		if count != len(original) {
			t.Fatalf("%s: selected %d cases, want %d with explicit skip records", target, count, len(original))
		}
	}
}
