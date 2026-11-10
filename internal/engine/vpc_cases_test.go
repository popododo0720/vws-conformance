package engine

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/vpc"
)

func TestVPCCasesAgainstLocalFake(t *testing.T) {
	group := vpc.Vpcs()
	count := 0
	for _, c := range group.Cases {
		if c.ID == "VPC-VPC-104" {
			continue
		}
		count++
		t.Run(c.ID, func(t *testing.T) {
			fake, rt := gateRuntime(t)
			result := runOne(context.Background(), rt, group, c)
			if result.Status != "pass" {
				t.Fatalf("case %s: %s %s", c.ID, result.Status, result.Reason)
			}
			expectedDeletes := 1
			if c.ID == "VPC-VPC-101" {
				expectedDeletes = 0
			}
			if c.ID == "VPC-VPC-102" {
				expectedDeletes = 2
			}
			if fake.Calls("DeleteVpc") != expectedDeletes {
				t.Fatalf("deletes %d, want %d", fake.Calls("DeleteVpc"), expectedDeletes)
			}
		})
	}
	if count != 5 {
		t.Fatalf("case count %d, want 5", count)
	}
}

func TestVPCCaseFailureStillCleansUp(t *testing.T) {
	fake, rt := gateRuntime(t)
	group := vpc.Vpcs()
	for _, c := range group.Cases {
		if c.ID != "VPC-VPC-001" {
			continue
		}
		c.Expect.HTTP = 418
		result := runOne(context.Background(), rt, group, c)
		if result.Status != "fail" {
			t.Fatalf("expected failed assertion, got %s: %s", result.Status, result.Reason)
		}
		if fake.Calls("DeleteVpc") != 1 {
			t.Fatalf("cleanup deletes %d", fake.Calls("DeleteVpc"))
		}
		return
	}
	t.Fatal("case missing")
}
