package engine

import (
	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// ReadOnlyGroups is the same selection used when executing the pve-test target.
func ReadOnlyGroups() []bind.Group {
	return selectGroups(harness.Config{Full: true, Target: "pve-test"}, "", "")
}

// These are the cases in docs/split-plan.md that need no resource setup.
// Filtering on the operation alone would admit Describe cases with mutating fixtures.
func readOnlyCases(all []cases.Case) []cases.Case {
	var out []cases.Case
	for _, c := range all {
		switch c.ID {
		case "EC2-DI-101", "EC2-DI-102", "EC2-DIA-101", "EC2-DIMG-001", "EC2-DIMG-101",
			"EC2-KP-103", "EC2-EIP-103", "EC2-GAP-003", "EC2-GAP-004", "EC2-GAP-008",
			"EBS-VOL-104", "EBS-SNAP-102", "EBS-GAP-002",
			"VPC-VPC-104", "VPC-SUB-103", "VPC-SG-104", "VPC-GAP-004", "VPC-GAP-005", "VPC-GAP-007",
			"AUTH-001", "AUTH-002", "AUTH-003", "AUTH-004", "AUTH-005", "AUTH-006", "AUTH-007",
			"AUTH-008", "AUTH-011", "AUTH-013":
			out = append(out, c)
		}
	}
	return out
}
