package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/iamfake"

	iamcases "github.com/popododo0720/vws-conformance/iam"
)

func mfaDeviceSerial(rt *harness.Runtime) string {
	return "arn:aws:iam::" + iamfake.Account + ":mfa/" + rt.Name("mfa-device")
}

// A re-run of the case in the same run creates the device again: the first run's cleanup
// deleted it, so the name is free and nothing is left behind either time.
func TestMFADeviceCasePassesOnRerunAndCleansUp(t *testing.T) {
	fake, rt := mfaRuntime(t)
	group := iamcases.MFADevices()
	c := group.Cases[0]
	for run := 1; run <= 2; run++ {
		result := runOne(context.Background(), rt, group, c)
		if result.Status != "pass" {
			t.Fatalf("run %d: case result = %+v", run, result)
		}
		if _, ok := fake.Device(mfaDeviceSerial(rt)); ok {
			t.Fatalf("run %d: case left %s behind", run, mfaDeviceSerial(rt))
		}
		raw, err := os.ReadFile(result.RawRef)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), iamfake.Seed) {
			t.Fatalf("run %d: raw capture holds the device seed: %s", run, raw)
		}
	}
	if got := fake.Calls("CreateVirtualMFADevice"); got != 2 {
		t.Fatalf("devices created = %d, want 2", got)
	}
}

// A device that already holds the case's serial number but another run's tag is refused by IAM
// (EntityAlreadyExists), so the case fails and its cleanup leaves that device alone.
func TestMFADeviceCaseFailsAndKeepsForeignDevice(t *testing.T) {
	fake, rt := mfaRuntime(t)
	serial := mfaDeviceSerial(rt)
	fake.PutDevice(serial, map[string]string{harness.RunTagKey: "conf-20261015T110000Z-ffffff"})
	group := iamcases.MFADevices()
	result := runOne(context.Background(), rt, group, group.Cases[0])
	if result.Status != "fail" || result.Actual["code"] != "EntityAlreadyExists" {
		t.Fatalf("case result = %+v", result)
	}
	if _, ok := fake.Device(serial); !ok {
		t.Fatalf("cleanup deleted the foreign device %s", serial)
	}
}

func TestMFADeviceCaseFailsWhenCreateIsRefused(t *testing.T) {
	fake, rt := mfaRuntime(t)
	fake.SetCreateHook(func() string { return "InvalidInput" })
	group := iamcases.MFADevices()
	result := runOne(context.Background(), rt, group, group.Cases[0])
	if result.Status != "fail" {
		t.Fatalf("case result = %+v", result)
	}
	if got := fake.Calls("DeleteVirtualMFADevice"); got != 0 {
		t.Fatalf("cleanup sent %d deletes for a device that was never created", got)
	}
}
