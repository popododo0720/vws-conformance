package ebs

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/ec2fake"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestSnapshotCasesAgainstLocalFake(t *testing.T) {
	fake := ec2fake.New()
	t.Cleanup(fake.Close)
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: "conf-20261110T000000Z-abc123",
		EC2Endpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"}, Target: "local-fake",
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	g := Snapshots()
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
			status, reason := expect.Judge(c.Expect, out)
			if status != "pass" || (runErr != nil && out.Code == "") {
				t.Fatalf("%s: %s; outcome=%+v error=%v", status, reason, out, runErr)
			}
			if v, s := fake.ResourceCounts(); v != 0 || s != 0 {
				t.Fatalf("left volumes=%d snapshots=%d", v, s)
			}
		})
	}
}
