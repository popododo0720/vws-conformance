package harness

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const iamAccount = "123456789012"

// iamFake is a loopback IAM query server for the managed-policy calls the gate makes.
type iamFake struct {
	*httptest.Server
	mu       sync.Mutex
	policies map[string]map[string]string
	calls    map[string]int
}

func newIAMFake(t *testing.T) *iamFake {
	t.Helper()
	f := &iamFake{policies: map[string]map[string]string{}, calls: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *iamFake) put(arn string, tags map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.policies[arn] = tags
}

func (f *iamFake) tags(arn string) (map[string]string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tags, ok := f.policies[arn]
	return tags, ok
}

func (f *iamFake) count(action string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[action]
}

func (f *iamFake) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	action, arn := r.Form.Get("Action"), r.Form.Get("PolicyArn")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[action]++
	tags, exists := f.policies[arn]
	body := ""
	switch {
	case action == "CreatePolicy":
		arn = fmt.Sprintf("arn:aws:iam::%s:policy/%s", iamAccount, r.Form.Get("PolicyName"))
		f.policies[arn] = map[string]string{}
		body = "<CreatePolicyResult><Policy><Arn>" + html.EscapeString(arn) +
			"</Arn></Policy></CreatePolicyResult>"
	case arn == "":
		iamError(w, http.StatusBadRequest, "ValidationError")
		return
	case !exists:
		iamError(w, http.StatusNotFound, "NoSuchEntity")
		return
	case action == "ListPolicyTags":
		var set strings.Builder
		for k, v := range tags {
			set.WriteString("<member><Key>" + html.EscapeString(k) +
				"</Key><Value>" + html.EscapeString(v) + "</Value></member>")
		}
		body = "<ListPolicyTagsResult><IsTruncated>false</IsTruncated><Tags>" + set.String() +
			"</Tags></ListPolicyTagsResult>"
	case action == "TagPolicy":
		for k, v := range formPairs(r, "Tags.member.%d.Key", "Tags.member.%d.Value") {
			tags[k] = v
		}
	case action == "UntagPolicy":
		for i := 1; r.Form.Get(fmt.Sprintf("TagKeys.member.%d", i)) != ""; i++ {
			delete(tags, r.Form.Get(fmt.Sprintf("TagKeys.member.%d", i)))
		}
	case action == "DeletePolicy":
		delete(f.policies, arn)
	}
	w.Header().Set("Content-Type", "text/xml")
	reply(w, `<%[1]sResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">%[2]s`+
		`<ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata></%[1]sResponse>`, action, body)
}

func formPairs(r *http.Request, keyFmt, valueFmt string) map[string]string {
	out := map[string]string{}
	for i := 1; r.Form.Get(fmt.Sprintf(keyFmt, i)) != ""; i++ {
		out[r.Form.Get(fmt.Sprintf(keyFmt, i))] = r.Form.Get(fmt.Sprintf(valueFmt, i))
	}
	return out
}

func reply(w http.ResponseWriter, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		fmt.Fprintln(os.Stderr, "iamFake:", err)
	}
}

func iamError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	reply(w, `<ErrorResponse><Error><Type>Sender</Type><Code>%s</Code><Message>m</Message></Error>`+
		`<RequestId>req-1</RequestId></ErrorResponse>`, code)
}

func newIAMGate(t *testing.T) (*iamFake, *Runtime) {
	t.Helper()
	fake := newIAMFake(t)
	cfg := Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1", RunID: testRun,
		IAMEndpoint: fake.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{iamAccount}, Target: "local-fake",
	}
	cl, err := Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return fake, NewRuntime(cfg, cl)
}

func policyArn(name string) string { return fmt.Sprintf("arn:aws:iam::%s:policy/%s", iamAccount, name) }

func createPolicy(t *testing.T, view *Runtime, name string) string {
	t.Helper()
	out, err := view.IAMCreatePolicy(context.Background(), &iam.CreatePolicyInput{
		PolicyName: aws.String(name), PolicyDocument: aws.String("{}"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return aws.ToString(out.Policy.Arn)
}

func TestPolicyCreateTagUntagAndCleanupDeletes(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-PTAG-001")
	name := rt.Name("ptag")
	arn := createPolicy(t, view, name)
	ctx := context.Background()
	if _, err := view.IAMTagPolicy(ctx, &iam.TagPolicyInput{
		PolicyArn: aws.String(arn),
		Tags: []iamtypes.Tag{
			{Key: aws.String("a"), Value: aws.String("1")}, {Key: aws.String("b"), Value: aws.String("2")},
		},
	}); err != nil {
		t.Fatal(err)
	}
	drop := &iam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: []string{"a"}}
	if _, err := view.IAMUntagPolicy(ctx, drop); err != nil {
		t.Fatal(err)
	}
	if tags, _ := fake.tags(arn); len(tags) != 1 || tags["b"] != "2" {
		t.Fatalf("server tags %v", tags)
	}
	if err := view.EndCase(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := fake.tags(arn); ok || fake.count("DeletePolicy") != 1 {
		t.Fatalf("present %v deletes %d", ok, fake.count("DeletePolicy"))
	}
}

func TestCreateRefusesNameWithoutRunID(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-PTAG-001")
	_, err := view.IAMCreatePolicy(context.Background(),
		&iam.CreatePolicyInput{PolicyName: aws.String("someone-elses"), PolicyDocument: aws.String("{}")})
	if err == nil || fake.count("CreatePolicy") != 0 {
		t.Fatalf("err %v, creates %d", err, fake.count("CreatePolicy"))
	}
}

func TestForeignPolicyIsPreserved(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-PTAG-001")
	ctx := context.Background()
	for name, tags := range map[string]map[string]string{
		"other-run": {RunTagKey: foreignRun}, "untagged": {},
	} {
		arn := policyArn(rt.Name(name))
		fake.put(arn, tags)
		in := &iam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: []string{"a"}}
		_, errDelete := view.IAMDeletePolicy(ctx, arn)
		_, errUntag := view.IAMUntagPolicy(ctx, in)
		_, errProbe := view.IAMProbeUntagPolicy(ctx, in)
		wantPreserve(t, errDelete, "")
		wantPreserve(t, errUntag, "")
		wantPreserve(t, errProbe, "probe target exists")
	}
	if n := fake.count("DeletePolicy") + fake.count("UntagPolicy"); n != 0 {
		t.Fatalf("%d mutations reached a foreign policy", n)
	}
}

func TestProbeRefusesPolicyNotNamedForRun(t *testing.T) {
	fake, rt := newIAMGate(t)
	view := begin(t, rt, "IAM-PTAG-105")
	arn := policyArn("someone-elses-policy")
	fake.put(arn, nil)
	_, err := view.IAMProbeUntagPolicy(context.Background(),
		&iam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: []string{"a"}})
	wantPreserve(t, err, "not named for this run")
	if fake.count("UntagPolicy")+fake.count("ListPolicyTags") != 0 {
		t.Fatal("a request was sent")
	}
}

// The SDK refuses a missing required member before sending; the probe must reach the service.
func TestProbeSendsWhatTheSDKWouldRefuse(t *testing.T) {
	for name, in := range map[string]*iam.UntagPolicyInput{
		"missing PolicyArn": {TagKeys: []string{"a"}},
		"absent run policy": {PolicyArn: aws.String(policyArn(NamePrefix(testRun) + "-gone")), TagKeys: []string{"a"}},
	} {
		t.Run(name, func(t *testing.T) {
			fake, rt := newIAMGate(t)
			view := begin(t, rt, "IAM-PTAG-101")
			if _, err := view.IAMProbeUntagPolicy(context.Background(), in); err == nil {
				t.Fatal("the fake rejects both, but no error came back")
			}
			if fake.count("UntagPolicy") != 1 {
				t.Fatalf("UntagPolicy reached the server %d times, want 1", fake.count("UntagPolicy"))
			}
		})
	}
}
