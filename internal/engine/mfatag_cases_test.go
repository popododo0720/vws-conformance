package engine

import (
	"context"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/iamfake"

	iamcases "github.com/popododo0720/vws-conformance/iam"
)

func mfaRuntime(t *testing.T) (*iamfake.Server, *harness.Runtime) {
	t.Helper()
	fake := iamfake.New()
	t.Cleanup(fake.Close)
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: "conf-20261015T120000Z-abc123",
		IAMEndpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"}, AccountID: iamfake.Account,
		AllowedAccounts: []string{iamfake.Account}, Target: "local-fake", OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fake, harness.NewRuntime(cfg, cl)
}

func TestMFATagCasesPassAndCleanUp(t *testing.T) {
	fake, rt := mfaRuntime(t)
	group := iamcases.MFATags()
	serial := "arn:aws:iam::" + iamfake.Account + ":mfa/" + rt.Name("mtag")
	for _, c := range group.Cases {
		t.Run(c.ID, func(t *testing.T) {
			result := runOne(context.Background(), rt, group, c)
			if result.Status != "pass" {
				t.Fatalf("case result = %+v", result)
			}
			if _, ok := fake.Device(serial); ok {
				t.Fatalf("case left %s behind", serial)
			}
		})
	}
	if got := fake.Calls("CreateVirtualMFADevice"); got != len(group.Cases)-1 {
		t.Fatalf("devices created = %d, want one per case but IAM-MTAG-103", got)
	}
}

func TestMFATagCasesFailWhenDeviceSetupIsRefused(t *testing.T) {
	fake, rt := mfaRuntime(t)
	fake.SetCreateHook(func() string { return "InvalidInput" })
	group := iamcases.MFATags()
	for _, c := range group.Cases {
		if c.ID == "IAM-MTAG-103" {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			result := runOne(context.Background(), rt, group, c)
			if result.Status != "fail" {
				t.Fatalf("case result = %+v", result)
			}
		})
	}
}
