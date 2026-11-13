package ec2

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/ec2fake"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestInstanceCasesAgainstLocalFake(t *testing.T) {
	fake := ec2fake.New()
	t.Cleanup(fake.Close)
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: "conf-20261113T000000Z-abc123",
		EC2Endpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"}, Target: "local-fake",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	g := Instances()
	if len(g.Cases) != 14 {
		t.Fatalf("registered %d instance cases, want 14", len(g.Cases))
	}
	for _, c := range g.Cases {
		t.Run(c.ID, func(t *testing.T) {
			view, err := rt.BeginCase(c.ID)
			if err != nil {
				t.Fatal(err)
			}
			out, runErr := g.Run(context.Background(), view, c)
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if status, reason := expect.Judge(c.Expect, out); status != "pass" || runErr != nil {
				t.Fatalf("%s: %s; outcome=%+v error=%v", status, reason, out, runErr)
			}
			before := fake.Calls("TerminateInstances")
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if fake.Calls("TerminateInstances") != before {
				t.Fatal("repeated cleanup terminated an instance again")
			}
		})
	}
	if fake.Calls("TerminateInstances") != 11 {
		t.Fatalf("termination calls = %d, want 11 (including the idempotent launch replay)", fake.Calls("TerminateInstances"))
	}
}

func TestInstanceCasesRegistered(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range Instances().Cases {
		registered[c.ID] = true
	}
	for _, id := range []string{
		"EC2-RUN-001", "EC2-RUN-101", "EC2-RUN-110", "EC2-RUN-201", "EC2-RUN-202",
		"EC2-DI-001", "EC2-DI-201", "EC2-DIA-001", "EC2-STOP-001", "EC2-START-001", "EC2-TERM-001",
	} {
		if !registered[id] {
			t.Errorf("required case %s is absent", id)
		}
	}
}
