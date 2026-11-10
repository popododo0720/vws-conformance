package engine

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/vpc"
)

func TestInternetGatewayCaseAgainstLocalFake(t *testing.T) {
	fake, rt := gateRuntime(t)
	group := vpc.Gaps()
	for _, c := range group.Cases {
		if c.ID != "VPC-GAP-006" {
			continue
		}
		result := runOne(context.Background(), rt, group, c)
		if result.Status != "pass" {
			t.Fatalf("case %s: %s %s", c.ID, result.Status, result.Reason)
		}
		if fake.Calls("CreateInternetGateway") != 1 || fake.Calls("DeleteInternetGateway") != 1 {
			t.Fatalf("create %d delete %d", fake.Calls("CreateInternetGateway"), fake.Calls("DeleteInternetGateway"))
		}
		return
	}
	t.Fatal("VPC-GAP-006 is not registered")
}

func TestInternetGatewayCaseFailureStillCleansUp(t *testing.T) {
	fake, rt := gateRuntime(t)
	group := vpc.Gaps()
	for _, c := range group.Cases {
		if c.ID != "VPC-GAP-006" {
			continue
		}
		c.Expect.HTTP = 418
		result := runOne(context.Background(), rt, group, c)
		if result.Status != "fail" {
			t.Fatalf("expected failed assertion, got %s: %s", result.Status, result.Reason)
		}
		if fake.Calls("DeleteInternetGateway") != 1 {
			t.Fatalf("cleanup deletes %d", fake.Calls("DeleteInternetGateway"))
		}
		return
	}
	t.Fatal("case missing")
}
