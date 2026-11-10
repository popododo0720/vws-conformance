package harness

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
)

// versionSet is the fake's version list of one policy, oldest first.
type versionSet struct {
	ids     []string
	def     string
	next    int
	deleted []string
}

// serveVersion answers the policy-version actions under f.mu. It reports false when it wrote an
// error response.
func (f *iamFake) serveVersion(w http.ResponseWriter, r *http.Request, action, arn string) (string, bool) {
	vs := f.versions[arn]
	if vs == nil {
		vs = &versionSet{ids: []string{"v1"}, def: "v1", next: 2}
		f.versions[arn] = vs
	}
	id := r.Form.Get("VersionId")
	switch action {
	case "CreatePolicyVersion":
		id = "v" + strconv.Itoa(vs.next)
		vs.next++
		vs.ids = append(vs.ids, id)
		if r.Form.Get("SetAsDefault") == "true" {
			vs.def = id
		}
		return "<CreatePolicyVersionResult><PolicyVersion><VersionId>" + id + "</VersionId><IsDefaultVersion>" +
			strconv.FormatBool(vs.def == id) + "</IsDefaultVersion></PolicyVersion></CreatePolicyVersionResult>", true
	case "ListPolicyVersions":
		start := 0
		if m := r.Form.Get("Marker"); m != "" {
			var err error
			if start, err = strconv.Atoi(m); err != nil {
				iamError(w, http.StatusBadRequest, "InvalidInput")
				return "", false
			}
		}
		end := len(vs.ids)
		if f.pageSize > 0 {
			end = min(start+f.pageSize, end)
		}
		var members strings.Builder
		for _, v := range vs.ids[start:end] {
			members.WriteString("<member><VersionId>" + v + "</VersionId><IsDefaultVersion>" +
				strconv.FormatBool(v == vs.def) + "</IsDefaultVersion></member>")
		}
		page := "<IsTruncated>false</IsTruncated>"
		if end < len(vs.ids) {
			page = "<IsTruncated>true</IsTruncated><Marker>" + strconv.Itoa(end) + "</Marker>"
		}
		return "<ListPolicyVersionsResult><Versions>" + members.String() + "</Versions>" + page +
			"</ListPolicyVersionsResult>", true
	}
	i := slices.Index(vs.ids, id)
	switch {
	case i < 0:
		iamError(w, http.StatusNotFound, "NoSuchEntity")
		return "", false
	case action == "SetDefaultPolicyVersion":
		vs.def = id
	case id == vs.def:
		iamError(w, http.StatusConflict, "DeleteConflict")
		return "", false
	default:
		vs.ids = slices.Delete(vs.ids, i, i+1)
		vs.deleted = append(vs.deleted, id)
	}
	return "", true
}

func (f *iamFake) deletedVersions(arn string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if vs := f.versions[arn]; vs != nil {
		return vs.deleted
	}
	return nil
}

func addVersion(t *testing.T, view *Runtime, arn string, isDefault bool) string {
	t.Helper()
	out, err := view.IAMCreatePolicyVersion(context.Background(), &iam.CreatePolicyVersionInput{
		PolicyArn: aws.String(arn), PolicyDocument: aws.String("{}"), SetAsDefault: isDefault,
	})
	if err != nil {
		t.Fatal(err)
	}
	return aws.ToString(out.PolicyVersion.VersionId)
}

// DeletePolicy answers DeleteConflict while a non-default version is left, so cleanup must delete
// those first and keep the default, which goes with the policy.
func TestPolicyCleanupDeletesNonDefaultVersionsFirst(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-GAP-004")
	arn := createPolicy(t, view, rt.Name("pol"))
	addVersion(t, view, arn, false)
	v3 := addVersion(t, view, arn, false)
	if _, err := view.IAMSetDefaultPolicyVersion(context.Background(), &iam.SetDefaultPolicyVersionInput{
		PolicyArn: aws.String(arn), VersionId: aws.String(v3),
	}); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.deletedVersions(arn); !slices.Equal(got, []string{"v1", "v2"}) {
		t.Fatalf("deleted versions %v, want [v1 v2]", got)
	}
	if _, ok := fake.tags(arn); ok || fake.count("DeletePolicy") != 1 {
		t.Fatalf("present %v deletes %d", ok, fake.count("DeletePolicy"))
	}
}

// AWS allows five versions; with one per page cleanup must follow every marker.
func TestPolicyCleanupFollowsVersionPages(t *testing.T) {
	fake, rt := newIAMGate(t)
	fake.pageSize = 1
	view := begin(t, rt, "IAM-GAP-001")
	arn := createPolicy(t, view, rt.Name("pol"))
	for range 4 {
		addVersion(t, view, arn, false)
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.deletedVersions(arn); len(got) != 4 || fake.count("ListPolicyVersions") != 5 {
		t.Fatalf("deleted %v after %d list pages, want 4 after 5", got, fake.count("ListPolicyVersions"))
	}
	if _, ok := fake.tags(arn); ok {
		t.Fatal("policy left behind")
	}
}

// A version the case already deleted is not deleted again, and a policy the case already deleted
// gets no second delete.
func TestPolicyCleanupAfterTheCaseDeleted(t *testing.T) {
	t.Run("a version", func(t *testing.T) {
		fake, rt := newIAMGate(t)
		view := begin(t, rt, "IAM-GAP-003")
		arn := createPolicy(t, view, rt.Name("pol"))
		v2 := addVersion(t, view, arn, false)
		if _, err := view.IAMDeletePolicyVersion(context.Background(), &iam.DeletePolicyVersionInput{
			PolicyArn: aws.String(arn), VersionId: aws.String(v2),
		}); err != nil {
			t.Fatal(err)
		}
		if err := view.EndCase(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fake.count("DeletePolicyVersion") != 1 || fake.count("DeletePolicy") != 1 {
			t.Fatalf("version deletes %d, policy deletes %d, want 1 and 1",
				fake.count("DeletePolicyVersion"), fake.count("DeletePolicy"))
		}
	})
	t.Run("the policy", func(t *testing.T) {
		fake, rt := newIAMGate(t)
		view := begin(t, rt, "IAM-H-DeletePolicy")
		arn := createPolicy(t, view, rt.Name("pol"))
		if _, err := view.IAMDeletePolicy(context.Background(), arn); err != nil {
			t.Fatal(err)
		}
		if err := view.EndCase(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fake.count("DeletePolicy") != 1 || fake.count("ListPolicyVersions") != 0 {
			t.Fatalf("policy deletes %d, version lists %d, want 1 and 0",
				fake.count("DeletePolicy"), fake.count("ListPolicyVersions"))
		}
	})
}

func TestForeignPolicyVersionsArePreserved(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-GAP-001")
	ctx := context.Background()
	for name, tags := range map[string]map[string]string{
		"other-run": {RunTagKey: foreignRun}, "untagged": {},
	} {
		arn := aws.String(policyArn(rt.Name(name)))
		fake.put(*arn, tags)
		_, errCreate := view.IAMCreatePolicyVersion(ctx,
			&iam.CreatePolicyVersionInput{PolicyArn: arn, PolicyDocument: aws.String("{}")})
		_, errDefault := view.IAMSetDefaultPolicyVersion(ctx,
			&iam.SetDefaultPolicyVersionInput{PolicyArn: arn, VersionId: aws.String("v1")})
		_, errDelete := view.IAMDeletePolicyVersion(ctx,
			&iam.DeletePolicyVersionInput{PolicyArn: arn, VersionId: aws.String("v1")})
		_, errProbe := view.IAMProbeDeletePolicy(ctx, *arn)
		wantPreserve(t, errCreate, "")
		wantPreserve(t, errDefault, "")
		wantPreserve(t, errDelete, "")
		wantPreserve(t, errProbe, "probe target exists")
	}
	for _, action := range []string{
		"CreatePolicyVersion", "SetDefaultPolicyVersion", "DeletePolicyVersion", "DeletePolicy",
	} {
		if n := fake.count(action); n != 0 {
			t.Errorf("%s reached a foreign policy %d times", action, n)
		}
	}
}

func TestProbeDeletePolicy(t *testing.T) {
	fake, rt := newIAMGate(t)
	rt.Cfg.AccountID = iamAccount
	view := begin(t, rt, "IAM-E-DeletePolicy")
	ctx := context.Background()
	_, err := view.IAMProbeDeletePolicy(ctx, policyArn("someone-elses-policy"))
	wantPreserve(t, err, "not named for this run")
	_, err = view.IAMProbeDeletePolicy(ctx, policyArn(rt.Name("gone")))
	var api smithy.APIError
	if !errors.As(err, &api) || api.ErrorCode() != "NoSuchEntity" {
		t.Fatalf("err %v, want NoSuchEntity from the service", err)
	}
	if fake.count("DeletePolicy") != 1 {
		t.Fatalf("DeletePolicy reached the server %d times, want 1", fake.count("DeletePolicy"))
	}
}

func TestNilPolicyVersionInputsReturnErrors(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-GAP-001")
	ctx := context.Background()
	if _, err := view.IAMCreatePolicyVersion(ctx, nil); err == nil {
		t.Error("IAMCreatePolicyVersion(nil) returned no error")
	}
	if _, err := view.IAMSetDefaultPolicyVersion(ctx, nil); err == nil {
		t.Error("IAMSetDefaultPolicyVersion(nil) returned no error")
	}
	if _, err := view.IAMDeletePolicyVersion(ctx, nil); err == nil {
		t.Error("IAMDeletePolicyVersion(nil) returned no error")
	}
	if n := fake.count("ListPolicyTags"); n != 0 {
		t.Errorf("%d observations were sent for nil inputs", n)
	}
}
