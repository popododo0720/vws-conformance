package engine

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/ec2"
)

func TestInstanceAssertionFailureCleansUp(t *testing.T) {
	fake, rt := gateRuntime(t)
	g := ec2.Instances()
	for _, c := range g.Cases {
		if c.ID != "EC2-RUN-001" {
			continue
		}
		c.Expect.HTTP = 201
		res := runOne(context.Background(), rt, g, c)
		if res.Status != "fail" {
			t.Fatalf("expected assertion failure, got %+v", res)
		}
		if fake.Calls("RunInstances") != 1 || fake.Calls("TerminateInstances") != 1 {
			t.Fatal("failed instance assertion did not terminate its owned launch")
		}
		return
	}
	t.Fatal("RunInstances success case is absent")
}
