package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/ec2fake"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func gateRuntime(t *testing.T) (*ec2fake.Server, *harness.Runtime) {
	t.Helper()
	fake := ec2fake.New()
	t.Cleanup(fake.Close)
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: "conf-20261015T120000Z-abc123",
		EC2Endpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"}, Target: "local-fake", OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fake, harness.NewRuntime(cfg, cl)
}

func TestRunOneCleansUpOnEveryExit(t *testing.T) {
	tests := []struct {
		name    string
		retag   bool
		end     func() (cases.Outcome, error)
		reasons []string
	}{
		{
			"case error", false, func() (cases.Outcome, error) { return cases.Outcome{}, errors.New("parse failed") },
			[]string{"parse failed"},
		},
		{"case panic", false, func() (cases.Outcome, error) { panic("boom") }, []string{"boom"}},
		{
			"cleanup preserve", true, func() (cases.Outcome, error) { return cases.Outcome{}, errors.New("parse failed") },
			[]string{"cleanup: ", "foreign owner", "case error: parse failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake, rt := gateRuntime(t)
			var id string
			g := bind.Group{Run: func(ctx context.Context, crt *harness.Runtime, _ cases.Case) (cases.Outcome, error) {
				out, err := crt.EC2CreateVolume(ctx, &ec2.CreateVolumeInput{
					AvailabilityZone: aws.String("vws-1a"), Size: aws.Int32(harness.VolumeGiB),
				})
				if err != nil {
					return cases.Outcome{}, fmt.Errorf("create: %w", err)
				}
				id = aws.ToString(out.VolumeId)
				if tt.retag {
					fake.PutVolume(id, map[string]string{harness.RunTagKey: "conf-20261104T000000Z-def456"})
				}
				return tt.end()
			}}
			res := runOne(context.Background(), rt, g, cases.Case{ID: "EBS-VOL-001"})
			if res.Status != "error" {
				t.Fatalf("status %s", res.Status)
			}
			for _, want := range tt.reasons {
				if !strings.Contains(res.Reason, want) {
					t.Fatalf("reason %q, want %q", res.Reason, want)
				}
			}
			_, present := fake.Volume(id)
			if id == "" || present != tt.retag {
				t.Fatalf("volume %q present %v", id, present)
			}
		})
	}
}
