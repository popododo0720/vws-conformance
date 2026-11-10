package harness

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"
)

// roleFake is a loopback IAM query server for the role calls the gate makes. roles maps a role
// name to its run tag value, "" for none.
type roleFake struct {
	*httptest.Server
	mu        sync.Mutex
	roles     map[string]string
	calls     map[string]int
	createErr int    // when set, CreateRole stores the role and then answers this status
	answerAs  string // when set, CreateRole answers this role name instead of the requested one
}

func newRoleGate(t *testing.T) (*roleFake, *Runtime) {
	t.Helper()
	f := &roleFake{roles: map[string]string{}, calls: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	cfg := Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: testRun,
		IAMEndpoint: f.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{iamAccount}, Target: "local-fake",
	}
	cl, err := Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return f, NewRuntime(cfg, cl)
}

func (f *roleFake) put(name, runTag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.roles[name] = runTag
}

func (f *roleFake) has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.roles[name]
	return ok
}

func (f *roleFake) count(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[action]
}

func (f *roleFake) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action, name := r.Form.Get("Action"), r.Form.Get("RoleName")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[action]++
	tag, exists := f.roles[name]
	body := ""
	switch {
	case action == "CreateRole" && exists:
		iamError(w, http.StatusConflict, "EntityAlreadyExists")
		return
	case action == "CreateRole":
		f.roles[name] = ""
		if f.createErr != 0 {
			iamError(w, f.createErr, "ServiceFailure")
			return
		}
		if f.answerAs != "" {
			name = f.answerAs
		}
		body = "<CreateRoleResult>" + roleXML(name, "") + "</CreateRoleResult>"
	case !exists:
		iamError(w, http.StatusNotFound, "NoSuchEntity")
		return
	case action == "GetRole":
		body = "<GetRoleResult>" + roleXML(name, tag) + "</GetRoleResult>"
	case action == "DeleteRole":
		delete(f.roles, name)
	}
	w.Header().Set("Content-Type", "text/xml")
	reply(w, `<%[1]sResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">%[2]s`+
		`<ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata></%[1]sResponse>`, action, body)
}

func roleXML(name, runTag string) string {
	tags := ""
	if runTag != "" {
		tags = "<Tags><member><Key>" + RunTagKey + "</Key><Value>" + html.EscapeString(runTag) +
			"</Value></member></Tags>"
	}
	return "<Role><RoleName>" + html.EscapeString(name) + "</RoleName><Arn>arn:aws:iam::" + iamAccount +
		":role/" + html.EscapeString(name) + "</Arn>" + tags + "</Role>"
}

func createRole(ctx context.Context, view *Runtime, name string) error {
	_, err := view.IAMCreateRole(ctx, &iam.CreateRoleInput{
		RoleName: aws.String(name), AssumeRolePolicyDocument: aws.String("{}"),
	})
	return err
}

func TestRoleCreateDeleteAndCleanup(t *testing.T) {
	fake, rt := newRoleGate(t)
	ctx := context.Background()
	view := begin(t, rt, "IAM-ROLE-001")
	kept, deleted := rt.Name("role-kept"), rt.Name("role-deleted")
	for _, name := range []string{kept, deleted} {
		if err := createRole(ctx, view, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := view.IAMDeleteRole(ctx, deleted); err != nil {
		t.Fatal(err)
	}
	if err := view.EndCase(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.has(kept) || fake.has(deleted) || fake.count("DeleteRole") != 2 {
		t.Fatalf("kept %v deleted %v, DeleteRole sent %d times, want 2",
			fake.has(kept), fake.has(deleted), fake.count("DeleteRole"))
	}
}

func TestRoleCreateRefusesNameWithoutRunID(t *testing.T) {
	fake, rt := newRoleGate(t)
	view := begin(t, rt, "IAM-ROLE-001")
	if err := createRole(context.Background(), view, "someone-elses"); err == nil || fake.count("CreateRole") != 0 {
		t.Fatalf("err %v, creates %d", err, fake.count("CreateRole"))
	}
	if _, err := view.IAMCreateRole(context.Background(), nil); err == nil {
		t.Fatal("IAMCreateRole(nil) returned no error")
	}
}

// A re-run of the same create is refused by the service; cleanup still removes the first role once.
func TestRoleDuplicateCreateIsRefusedAndCleanedOnce(t *testing.T) {
	fake, rt := newRoleGate(t)
	ctx := context.Background()
	view := begin(t, rt, "IAM-ROLE-001")
	name := rt.Name("role-dup")
	if err := createRole(ctx, view, name); err != nil {
		t.Fatal(err)
	}
	var api smithy.APIError
	if err := createRole(ctx, view, name); !errors.As(err, &api) || api.ErrorCode() != "EntityAlreadyExists" {
		t.Fatalf("second create err %v, want EntityAlreadyExists", err)
	}
	if err := view.EndCase(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.has(name) || fake.count("DeleteRole") != 1 {
		t.Fatalf("present %v, DeleteRole sent %d times, want 1", fake.has(name), fake.count("DeleteRole"))
	}
}

// A create cut off by a 5xx (a crash or restart on the server side) has an unknown outcome, and a
// create answered with a name other than the one sent cannot be trusted. Neither is ever deleted.
func TestRoleUnknownOrMismatchedCreateIsPreserved(t *testing.T) {
	for name, set := range map[string]func(*roleFake){
		"server failure": func(f *roleFake) { f.createErr = http.StatusInternalServerError },
		"other name":     func(f *roleFake) { f.answerAs = "victim" },
	} {
		t.Run(name, func(t *testing.T) {
			fake, rt := newRoleGate(t)
			set(fake)
			fake.put("victim", "")
			ctx := context.Background()
			view := begin(t, rt, "IAM-ROLE-001")
			if err := createRole(ctx, view, rt.Name("role-lost")); err == nil {
				t.Fatal("create returned no error")
			}
			wantPreserve(t, view.EndCase(ctx), "create outcome unknown")
			if fake.count("DeleteRole") != 0 || !fake.has("victim") {
				t.Fatalf("DeleteRole sent %d times, victim present %v", fake.count("DeleteRole"), fake.has("victim"))
			}
		})
	}
}

func TestForeignRoleIsPreserved(t *testing.T) {
	fake, rt := newRoleGate(t)
	ctx := context.Background()
	view := begin(t, rt, "IAM-E-DeleteRole")
	for _, name := range []string{rt.Name("other-run"), rt.Name("untagged")} {
		tag := ""
		if name == rt.Name("other-run") {
			tag = foreignRun
		}
		fake.put(name, tag)
		_, errDelete := view.IAMDeleteRole(ctx, name)
		wantPreserve(t, errDelete, "")
		_, errProbe := view.IAMProbeDeleteRole(ctx, name)
		wantPreserve(t, errProbe, "not proven absent")
	}
	if fake.count("DeleteRole") != 0 {
		t.Fatalf("%d deletes reached a foreign role", fake.count("DeleteRole"))
	}
}

func TestRoleProbe(t *testing.T) {
	fake, rt := newRoleGate(t)
	ctx := context.Background()
	view := begin(t, rt, "IAM-E-DeleteRole")
	fake.put("someone-elses", "")
	_, err := view.IAMProbeDeleteRole(ctx, "someone-elses")
	wantPreserve(t, err, "not named for this run")
	if fake.count("GetRole")+fake.count("DeleteRole") != 0 {
		t.Fatal("a request was sent for a role not named for this run")
	}
	var api smithy.APIError
	if _, err := view.IAMProbeDeleteRole(ctx, rt.Name("role-gone")); !errors.As(err, &api) ||
		api.ErrorCode() != "NoSuchEntity" {
		t.Fatalf("err %v, want NoSuchEntity from the service", err)
	}
	if fake.count("DeleteRole") != 1 {
		t.Fatalf("DeleteRole reached the server %d times, want 1", fake.count("DeleteRole"))
	}
}
