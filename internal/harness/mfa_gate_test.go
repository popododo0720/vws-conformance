package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	"github.com/popododo0720/vws-conformance/internal/iamfake"
)

func newMFAGate(t *testing.T) (*iamfake.Server, *Runtime) {
	t.Helper()
	fake := iamfake.New()
	t.Cleanup(fake.Close)
	cfg := Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: testRun,
		IAMEndpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{iamfake.Account}, Target: "local-fake",
	}
	cl, err := Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fake, NewRuntime(cfg, cl)
}

func createMFADevice(t *testing.T, view *Runtime, name string) string {
	t.Helper()
	out, err := view.IAMCreateVirtualMFADevice(context.Background(),
		&iam.CreateVirtualMFADeviceInput{VirtualMFADeviceName: aws.String(name)})
	if err != nil {
		t.Fatal(err)
	}
	return aws.ToString(out.VirtualMFADevice.SerialNumber)
}

func nameTag(value string) []iamtypes.Tag {
	return []iamtypes.Tag{{Key: aws.String("Name"), Value: aws.String(value)}}
}

func TestMFAGateTagsRunAndCleansUp(t *testing.T) {
	fake, rt := newMFAGate(t)
	view := begin(t, rt, "IAM-MTAG-001")
	serial := createMFADevice(t, view, "conf-mfa")
	if got, _ := fake.Device(serial); got[RunTagKey] != testRun {
		t.Fatalf("created tags = %v", got)
	}
	for range 2 {
		if _, err := view.IAMTagMFADevice(context.Background(), serial, nameTag("example")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := view.IAMUntagMFADevice(context.Background(), serial, []string{"Name"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := fake.Device(serial); got[RunTagKey] != testRun || got["Name"] != "" {
		t.Fatalf("tags after untag = %v", got)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Device(serial); ok {
		t.Fatal("cleanup left the device behind")
	}
	if _, err := view.IAMTagMFADevice(context.Background(), serial, nameTag("late")); err == nil {
		t.Fatal("closed scope accepted a tag write")
	}
}

func TestMFAGateRefusesRunTagKey(t *testing.T) {
	fake, rt := newMFAGate(t)
	view := begin(t, rt, "IAM-MTAG-107")
	forged := []iamtypes.Tag{{Key: aws.String("VWS-Conformance:Run"), Value: aws.String(foreignRun)}}
	if _, err := view.IAMCreateVirtualMFADevice(context.Background(), &iam.CreateVirtualMFADeviceInput{
		VirtualMFADeviceName: aws.String("conf-forged"), Tags: forged,
	}); err == nil {
		t.Fatal("create with a forged run tag accepted")
	}
	serial := createMFADevice(t, view, "conf-mfa")
	if _, err := view.IAMTagMFADevice(context.Background(), serial, forged); err == nil {
		t.Fatal("tag with a run tag spelling accepted")
	}
	if _, err := view.IAMUntagMFADevice(context.Background(), serial, []string{RunTagKey}); err == nil {
		t.Fatal("untag of the run tag accepted")
	}
	if fake.Calls("CreateVirtualMFADevice") != 1 || fake.Calls("TagMFADevice") != 0 || fake.Calls("UntagMFADevice") != 0 {
		t.Fatal("an ownership mutation reached the SDK")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// A serial number names a device, not one creation of it: a device another run makes again under
// a serial this case deleted is foreign, and an untagged device is no one's.
func TestMFAGatePreservesForeignAndUntaggedDevices(t *testing.T) {
	fake, rt := newMFAGate(t)
	view := begin(t, rt, "IAM-MTAG-101")
	serial := createMFADevice(t, view, "conf-mfa")
	if _, err := view.IAMDeleteVirtualMFADevice(context.Background(), serial); err != nil {
		t.Fatal(err)
	}
	fake.PutDevice(serial, map[string]string{RunTagKey: foreignRun})
	untagged := "arn:aws:iam::" + iamfake.Account + ":mfa/someone"
	fake.PutDevice(untagged, map[string]string{})
	for _, target := range []string{serial, untagged} {
		_, err := view.IAMTagMFADevice(context.Background(), target, nameTag("x"))
		wantPreserve(t, err, "")
		_, err = view.IAMUntagMFADevice(context.Background(), target, []string{"Name"})
		wantPreserve(t, err, "")
		_, err = view.IAMDeleteVirtualMFADevice(context.Background(), target)
		wantPreserve(t, err, "")
	}
	if fake.Calls("TagMFADevice") != 0 || fake.Calls("UntagMFADevice") != 0 || fake.Calls("DeleteVirtualMFADevice") != 1 {
		t.Fatal("a mutation of a device this run does not own reached the SDK")
	}
	if err := view.EndCase(context.Background()); err == nil {
		t.Fatal("cleanup of a serial re-made by another run reported success")
	}
	if got, ok := fake.Device(serial); !ok || got[RunTagKey] != foreignRun {
		t.Fatalf("foreign device = %v %v", got, ok)
	}
}

func TestMFAGateShutdownDeletesLiveDevices(t *testing.T) {
	fake, rt := newMFAGate(t)
	view := begin(t, rt, "IAM-MTAG-001")
	serial := createMFADevice(t, view, "conf-mfa")
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Device(serial); ok {
		t.Fatal("shutdown left the device behind")
	}
}

func TestMFAGateRefusedCreateLeavesNothing(t *testing.T) {
	fake, rt := newMFAGate(t)
	fake.SetCreateHook(func() string { return "InvalidInput" })
	view := begin(t, rt, "IAM-MTAG-001")
	if _, err := view.IAMCreateVirtualMFADevice(context.Background(),
		&iam.CreateVirtualMFADeviceInput{VirtualMFADeviceName: aws.String("conf-mfa")}); err == nil {
		t.Fatal("refused create reported success")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.Calls("ListMFADeviceTags") != 0 || fake.Calls("DeleteVirtualMFADevice") != 0 {
		t.Fatal("cleanup touched a device that was never created")
	}
}
