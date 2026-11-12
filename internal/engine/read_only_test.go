package engine

import (
	"slices"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestReadOnlyCases(t *testing.T) {
	counts := map[string]int{}
	for _, g := range Groups() {
		for _, c := range readOnlyCases(g.Cases) {
			counts[g.Service]++
			if c.Skip == "" && g.Run == nil {
				t.Fatalf("%s has no runner", c.ID)
			}
		}
	}
	for service, want := range map[string]int{"ec2": 10, "ebs": 3, "vpc": 6, "auth": 10} {
		if counts[service] != want {
			t.Fatalf("%s: got %d cases, want %d", service, counts[service], want)
		}
		delete(counts, service)
	}
	if len(counts) != 0 {
		t.Fatalf("unexpected services: %v", counts)
	}
}

func TestReadOnlySelection(t *testing.T) {
	previous := *flagReadOnly
	t.Cleanup(func() { *flagReadOnly = previous })
	*flagReadOnly = true
	cfg := harness.Config{Full: true}
	if len(selectGroups(cfg, "ec2", "tags")) != 0 {
		t.Fatal("selected a group with only mutating fixtures")
	}
	groups := selectGroups(cfg, "ebs", "volumes")
	if len(groups) != 1 || len(groups[0].Cases) != 1 || groups[0].Cases[0].ID != "EBS-VOL-104" {
		t.Fatalf("read-only volume selection: %v", groups)
	}
	*flagReadOnly = false
	if len(selectGroups(cfg, "ebs", "volumes")[0].Cases) <= 1 {
		t.Fatal("read-only selection modified the full suite")
	}
}

func TestReadOnlyCasesRejectMutationAndUnknownInput(t *testing.T) {
	input := []cases.Case{
		{ID: "EBS-VOL-002", Op: "DescribeVolumes"},
		{ID: "VPC-VPC-002", Op: "DescribeVpcs"},
		{ID: "EC2-TAG-001", Op: "CreateTags"},
		{ID: "unknown", Op: "DescribeInstances"},
		{ID: "EC2-DI-102", Op: "DescribeInstances"},
	}
	before := slices.Clone(input)
	for range 2 {
		got := readOnlyCases(input)
		if len(got) != 1 || got[0].ID != "EC2-DI-102" {
			t.Fatalf("selected mutating fixture or unknown case: %v", got)
		}
		if len(readOnlyCases(got)) != 1 {
			t.Fatal("filter is not idempotent")
		}
	}
	for i := range input {
		if input[i].ID != before[i].ID {
			t.Fatal("filter mutated the group")
		}
	}
	if len(readOnlyCases(nil)) != 0 {
		t.Fatal("empty input selected cases")
	}
}
