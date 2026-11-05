package harness

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/ec2fake"
)

const foreignRun = "conf-20261104T000000Z-def456"

func newGate(t *testing.T) (*ec2fake.Server, *Runtime) {
	t.Helper()
	fake := ec2fake.New()
	t.Cleanup(fake.Close)
	cfg := Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: testRun,
		EC2Endpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"}, Target: "local-fake",
	}
	cl, err := Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fake, NewRuntime(cfg, cl)
}

func volumeInput() *ec2.CreateVolumeInput {
	return &ec2.CreateVolumeInput{AvailabilityZone: aws.String("vws-1a"), Size: aws.Int32(VolumeGiB)}
}

func createVolume(t *testing.T, view *Runtime) string {
	t.Helper()
	out, err := view.EC2CreateVolume(context.Background(), volumeInput())
	if err != nil {
		t.Fatal(err)
	}
	return aws.ToString(out.VolumeId)
}

func wantPreserve(t *testing.T, err error, reason string) {
	t.Helper()
	var pe *PreserveError
	if !errors.As(err, &pe) || !strings.Contains(pe.Reason, reason) {
		t.Fatalf("err %v, want a PreserveError with %q", err, reason)
	}
}

func TestCreateTagsAndCleanupDeletes(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-001")
	in := volumeInput()
	in.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeVolume, Tags: []types.Tag{{Key: aws.String("Name"), Value: aws.String("v")}},
	}}
	out, err := view.EC2CreateVolume(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	id := aws.ToString(out.VolumeId)
	tags, ok := fake.Volume(id)
	if !ok || tags[RunTagKey] != testRun || tags["Name"] != "v" {
		t.Fatalf("server tags %v", tags)
	}
	if len(in.TagSpecifications) != 1 || len(in.TagSpecifications[0].Tags) != 1 {
		t.Fatalf("caller input was modified: %+v", in.TagSpecifications)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.Volume(id); ok || fake.Calls("DeleteVolume") != 1 {
		t.Fatalf("present %v deletes %d", ok, fake.Calls("DeleteVolume"))
	}
}

func TestCreateRefusesForgedRunTag(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-001")
	in := volumeInput()
	in.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeVolume,
		Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
	}}
	if _, err := view.EC2CreateVolume(context.Background(), in); err == nil {
		t.Fatal("forged run tag accepted")
	}
	if fake.Calls("CreateVolume") != 0 {
		t.Fatal("create was sent")
	}
}

func TestCleanupWaitsForCreateInFlight(t *testing.T) {
	for _, closer := range []string{"EndCase", "Shutdown"} {
		t.Run(closer, func(t *testing.T) {
			fake, rt := newGate(t)
			view := begin(t, rt, "EBS-VOL-001")
			entered := make(chan struct{})
			release := make(chan struct{})
			fake.SetCreateHook(func() ec2fake.CreateResult {
				close(entered)
				<-release
				return ec2fake.CreateResult{}
			})
			ctx, cancel := context.WithCancel(context.Background())
			created := make(chan error, 1)
			go func() {
				_, err := view.EC2CreateVolume(ctx, volumeInput())
				created <- err
			}()
			<-entered
			cancel()
			closed := make(chan error, 1)
			go func() {
				if closer == "EndCase" {
					closed <- view.EndCase(context.Background())
				} else {
					closed <- rt.Shutdown(context.Background())
				}
			}()
			select {
			case err := <-closed:
				t.Fatalf("%s returned %v while the create was in flight", closer, err)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if err := <-created; err != nil {
				t.Fatalf("a canceled case context aborted the create on the wire: %v", err)
			}
			if err := <-closed; err != nil {
				t.Fatal(err)
			}
			if fake.Calls("DeleteVolume") != 1 {
				t.Fatalf("deletes %d", fake.Calls("DeleteVolume"))
			}
			if _, err := view.EC2CreateVolume(context.Background(), volumeInput()); err == nil {
				t.Fatal("create admitted after close")
			}
			if _, err := view.EC2DeleteVolume(context.Background(), "vol-x"); err == nil {
				t.Fatal("delete admitted after close")
			}
			if fake.Calls("CreateVolume") != 1 || fake.Calls("DeleteVolume") != 1 {
				t.Fatalf("calls after close: create %d delete %d", fake.Calls("CreateVolume"), fake.Calls("DeleteVolume"))
			}
		})
	}
}

func TestExplicitDeleteThenCleanupIsAlreadyGone(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-002")
	id := createVolume(t, view)
	if _, err := view.EC2DeleteVolume(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.Calls("DeleteVolume") != 1 {
		t.Fatalf("deletes %d", fake.Calls("DeleteVolume"))
	}
}

func TestNegativeDeleteOfOurDeletedID(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-101")
	id := createVolume(t, view)
	if _, err := view.EC2DeleteVolume(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	o, err := view.Call(context.Background(), func(ctx context.Context) error {
		_, err := view.EC2DeleteVolume(ctx, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if o.Code != "InvalidVolume.NotFound" || o.HTTP != 400 || !strings.Contains(o.Raw, "req-DeleteVolume") {
		t.Fatalf("outcome %+v", o)
	}
	if fake.Calls("DeleteVolume") != 2 {
		t.Fatalf("deletes %d", fake.Calls("DeleteVolume"))
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.Calls("DeleteVolume") != 2 {
		t.Fatalf("cleanup sent a delete: %d", fake.Calls("DeleteVolume"))
	}
}

func TestForeignVolumeIsPreserved(t *testing.T) {
	fake, rt := newGate(t)
	fake.PutVolume("vol-foreign", map[string]string{RunTagKey: foreignRun})
	view := begin(t, rt, "EBS-VOL-102")
	o, err := view.Call(context.Background(), func(ctx context.Context) error {
		_, err := view.EC2DeleteVolume(ctx, "vol-foreign")
		return err
	})
	wantPreserve(t, err, "foreign owner")
	if o.HTTP != 0 || strings.Contains(o.Raw, "req-DescribeVolumes") {
		t.Fatalf("the observation leaked into the outcome: %+v", o)
	}
	if fake.Calls("DeleteVolume") != 0 {
		t.Fatal("foreign volume delete was sent")
	}
}

func TestRetaggedVolumeIsPreservedAtCleanup(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EBS-VOL-001")
	id := createVolume(t, view)
	fake.PutVolume(id, map[string]string{RunTagKey: foreignRun})
	wantPreserve(t, view.EndCase(context.Background()), "foreign owner")
	if _, ok := fake.Volume(id); !ok || fake.Calls("DeleteVolume") != 0 {
		t.Fatalf("present %v deletes %d", ok, fake.Calls("DeleteVolume"))
	}
}

func TestLostCreateIsReported(t *testing.T) {
	fake, rt := newGate(t)
	fake.SetCreateHook(func() ec2fake.CreateResult { return ec2fake.CreateResult{Drop: true} })
	view := begin(t, rt, "EBS-VOL-001")
	if _, err := view.EC2CreateVolume(context.Background(), volumeInput()); err == nil {
		t.Fatal("dropped create succeeded")
	}
	wantPreserve(t, view.EndCase(context.Background()), "create outcome unknown")
	if fake.Calls("DescribeVolumes") != 0 || fake.Calls("DeleteVolume") != 0 {
		t.Fatal("cleanup touched a volume with no id")
	}
}

func TestCreateIsNotRetriedAfterALostResponse(t *testing.T) {
	fake, rt := newGate(t)
	attempt := 0
	fake.SetCreateHook(func() ec2fake.CreateResult {
		attempt++
		if attempt == 1 {
			return ec2fake.CreateResult{Lose: true}
		}
		return ec2fake.CreateResult{Code: "RequestLimitExceeded"}
	})
	view := begin(t, rt, "EBS-VOL-001")
	if _, err := view.EC2CreateVolume(context.Background(), volumeInput()); err == nil {
		t.Fatal("lost create succeeded")
	}
	if n := fake.Calls("CreateVolume"); n != 1 {
		t.Fatalf("CreateVolume sent %d times, want 1", n)
	}
	wantPreserve(t, view.EndCase(context.Background()), "create outcome unknown")
}

func TestRefusedCreateNeedsNoCleanup(t *testing.T) {
	fake, rt := newGate(t)
	fake.SetCreateHook(func() ec2fake.CreateResult { return ec2fake.CreateResult{Code: "VolumeLimitExceeded"} })
	view := begin(t, rt, "EBS-VOL-103")
	o, err := view.Call(context.Background(), func(ctx context.Context) error {
		_, err := view.EC2CreateVolume(ctx, volumeInput())
		return err
	})
	if err != nil || o.Code != "VolumeLimitExceeded" {
		t.Fatalf("outcome %+v err %v", o, err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.Calls("DescribeVolumes") != 0 || fake.Calls("DeleteVolume") != 0 {
		t.Fatal("cleanup ran for a create the server refused")
	}
}

func TestRestartHasNoLedgerAuthority(t *testing.T) {
	fake, rt := newGate(t)
	fake.PutVolume("vol-untagged", map[string]string{})
	fake.PutVolume("vol-ours", map[string]string{RunTagKey: testRun})
	view := begin(t, rt, "EBS-VOL-002")
	_, err := view.EC2DeleteVolume(context.Background(), "vol-untagged")
	wantPreserve(t, err, "no ownership proof")
	if _, err := view.EC2DeleteVolume(context.Background(), "vol-ours"); err != nil {
		t.Fatalf("run-tagged volume: %v", err)
	}
	if _, ok := fake.Volume("vol-untagged"); !ok || fake.Calls("DeleteVolume") != 1 {
		t.Fatalf("untagged present %v deletes %d", ok, fake.Calls("DeleteVolume"))
	}
}

func TestUnscopedRuntimeSendsNothing(t *testing.T) {
	fake, rt := newGate(t)
	if _, err := rt.EC2CreateVolume(context.Background(), volumeInput()); err == nil {
		t.Fatal("create on the singleton runtime")
	}
	if _, err := rt.EC2DeleteVolume(context.Background(), "vol-x"); err == nil {
		t.Fatal("delete on the singleton runtime")
	}
	for _, action := range []string{"CreateVolume", "DescribeVolumes", "DeleteVolume"} {
		if fake.Calls(action) != 0 {
			t.Fatalf("%s sent", action)
		}
	}
}

func TestKindsCiteOfficialAbsenceDocs(t *testing.T) {
	for k, spec := range kinds {
		if spec.absence == "" || !strings.HasPrefix(spec.doc, "https://docs.aws.amazon.com/") ||
			spec.fetch == nil || spec.remove == nil {
			t.Fatalf("kind %s: %+v", k, spec)
		}
	}
}
