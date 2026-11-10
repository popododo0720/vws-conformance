package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/awsmodel"
	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/engine"
)

const fixtures = "../../internal/awsmodel/testdata/"

// fixtureConfig pins testdata/old.json as the ec2 model and serves testdata/new.json as the newer
// release; new.json has one edit per operation and none in SameOp.
func fixtureConfig(t *testing.T) config {
	t.Helper()
	base, current := t.TempDir(), t.TempDir()
	old, err := os.ReadFile(fixtures + "old.json")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(fixtures + "new.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "ec2.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "ec2.json"), updated, 0o600); err != nil {
		t.Fatal(err)
	}
	return config{
		Pin:     &pin{Botocore: "1.0.0", Models: []pinModel{{Service: "ec2", Path: "x", SHA256: digest(old)}}},
		Ops:     map[string][]string{"ec2": {"AddMember", "SameOp", "EnumOp"}},
		BaseDir: base, CurrentDir: current, Newest: "1.0.1",
	}
}

func findings(t *testing.T, cfg config) []finding {
	t.Helper()
	var out bytes.Buffer
	if err := run(cfg, &out); err != nil {
		t.Fatal(err)
	}
	var got []finding
	dec := json.NewDecoder(&out)
	for dec.More() {
		var f finding
		if err := dec.Decode(&f); err != nil {
			t.Fatal(err)
		}
		got = append(got, f)
	}
	return got
}

func TestImplementedOpsFollowTheCatalogue(t *testing.T) {
	ops, err := implementedOps(engine.Groups())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ svc, op string }{
		{"ec2", "DescribeInstances"},
		{"ec2", "DescribeVpcs"},
		{"ec2", "DescribeVolumes"},
		{"iam", "GetUser"},
		{"s3", "ListObjectsV2"},
		{"sts", "GetCallerIdentity"},
	} {
		if !slices.Contains(ops[want.svc], want.op) {
			t.Errorf("implemented %s ops %v lack %s", want.svc, ops[want.svc], want.op)
		}
	}
	if slices.Contains(ops["ec2"], "DescribeTags") {
		t.Errorf("gap case DescribeTags counted as implemented: %v", ops["ec2"])
	}
	_, err = implementedOps([]bind.Group{{Cases: []cases.Case{{Service: "lambda", Op: "Invoke", Impl: implemented}}}})
	if ue := (*UnmappedServiceError)(nil); !errors.As(err, &ue) || ue.Service != "lambda" {
		t.Errorf("unmapped service: err = %v, want UnmappedServiceError lambda", err)
	}
}

func TestRunPrintsOneFindingPerChangedOperation(t *testing.T) {
	got := findings(t, fixtureConfig(t))
	titles := make([]string, len(got))
	for i, f := range got {
		titles[i] = f.Title
		if !strings.HasPrefix(f.Body, f.Marker+"\n") {
			t.Errorf("%s: body does not start with its marker %q", f.Title, f.Marker)
		}
	}
	want := []string{
		"parity(awsmodel): ec2 AddMember changed in botocore 1.0.1",
		"parity(awsmodel): ec2 EnumOp changed in botocore 1.0.1",
	}
	if !slices.Equal(titles, want) {
		t.Fatalf("titles = %q, want %q", titles, want)
	}
	if !strings.Contains(got[0].Body, "\n+ input.Extra: string\n") {
		t.Errorf("AddMember body lacks the added member:\n%s", got[0].Body)
	}
}

// A re-run, including one after a run that stopped half way through opening issues, opens only
// the changes no issue records yet.
func TestRunSkipsChangesAlreadyFiled(t *testing.T) {
	cfg := fixtureConfig(t)
	all := findings(t, cfg)
	tests := []struct {
		name  string
		filed string
		want  int
	}{
		{"nothing filed", "", 2},
		{"first filed", "Title\n" + all[0].Body, 1},
		{"all filed", all[0].Body + all[1].Body, 0},
		{"other change of the same operation", "<!-- awsmodel-diff ec2 AddMember 0000000000000000 -->", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg.Filed = tt.filed
			if got := findings(t, cfg); len(got) != tt.want {
				t.Errorf("printed %d findings, want %d", len(got), tt.want)
			}
		})
	}
}

func TestRunRejectsUnpinnedInput(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*config)
		match func(error) bool
	}{
		{
			"baseline is not the pinned copy", func(c *config) { c.Pin.Models[0].SHA256 = strings.Repeat("0", 64) },
			func(err error) bool { de := (*DigestError)(nil); return errors.As(err, &de) },
		},
		{
			"newest is a branch", func(c *config) { c.Newest = "develop" },
			func(err error) bool { te := (*TagError)(nil); return errors.As(err, &te) },
		},
		{
			"implemented service missing from the pin", func(c *config) { c.Ops["iam"] = []string{"GetUser"} },
			func(err error) bool { return err != nil && strings.Contains(err.Error(), `"iam"`) },
		},
		{
			"implemented operation missing from the pin", func(c *config) { c.Ops["ec2"] = []string{"NoSuchOp"} },
			func(err error) bool { ue := (*awsmodel.UnknownOperationError)(nil); return errors.As(err, &ue) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fixtureConfig(t)
			tt.edit(&cfg)
			var out bytes.Buffer
			if err := run(cfg, &out); !tt.match(err) {
				t.Errorf("run() err = %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("printed findings before failing: %s", out.String())
			}
		})
	}
}

func TestParsePinRejectsMalformedPins(t *testing.T) {
	ec2 := `{"service":"ec2","path":"p","sha256":"` + strings.Repeat("a", 64) + `"}`
	tests := map[string]string{
		"branch":       `{"botocore":"develop","models":[` + ec2 + `]}`,
		"no models":    `{"botocore":"1.0.0","models":[]}`,
		"short sha":    `{"botocore":"1.0.0","models":[{"service":"ec2","path":"p","sha256":"abc"}]}`,
		"duplicate":    `{"botocore":"1.0.0","models":[` + ec2 + `,` + ec2 + `]}`,
		"extra fields": `{"botocore":"1.0.0","branch":"develop","models":[` + ec2 + `]}`,
	}
	for name, data := range tests {
		if _, err := parsePin([]byte(data)); err == nil {
			t.Errorf("%s: parsePin accepted %s", name, data)
		}
	}
}

// Moving the pin forward: -repin prints a pin.json that names the new tag and the sha256 of the
// newer files, so the next run accepts them as the baseline.
func TestRepinMovesThePinToTheNewerFiles(t *testing.T) {
	cfg := fixtureConfig(t)
	var out bytes.Buffer
	if err := repin(cfg, &out); err != nil {
		t.Fatal(err)
	}
	next, err := parsePin(out.Bytes())
	if err != nil {
		t.Fatalf("repin output does not parse: %v\n%s", err, out.String())
	}
	if next.Botocore != "1.0.1" {
		t.Errorf("botocore = %q, want 1.0.1", next.Botocore)
	}
	cfg.Pin, cfg.BaseDir = next, cfg.CurrentDir
	if got := findings(t, cfg); len(got) != 0 {
		t.Errorf("after the repin, %d findings remain", len(got))
	}
}
