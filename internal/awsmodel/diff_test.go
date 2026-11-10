package awsmodel

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func load(t *testing.T, path string) *Model {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse(%s): %v", path, err)
	}
	return m
}

// testdata/new.json is testdata/old.json with exactly one edit per operation below; SameOp only
// changes its documentation, which is not a change.
func TestDiffReportsEachKindOfChange(t *testing.T) {
	baseline := load(t, "testdata/old.json")
	current := load(t, "testdata/new.json")
	tests := []struct {
		op   string
		want []Change
	}{
		{"AddMember", []Change{{Operation: "AddMember", Added: []string{"input.Extra: string"}}}},
		{"RemoveMember", []Change{{Operation: "RemoveMember", Removed: []string{"input.Legacy: string"}}}},
		{"EnumOp", []Change{{Operation: "EnumOp", Added: []string{"input.State enum stopped"}}}},
		{"ErrorOp", []Change{{
			Operation: "ErrorOp",
			Added:     []string{"error LimitExceededException code=LimitExceeded http=409"},
		}}},
		{"RequiredOp", []Change{{Operation: "RequiredOp", Added: []string{"input.Name required"}}}},
		{"GoneOp", []Change{{Operation: "GoneOp", Removed: []string{"input: structure", "operation GoneOp"}}}},
		{"SameOp", nil},
	}
	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			got, err := Diff(baseline, current, []string{tt.op})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diff(%s) = %#v, want %#v", tt.op, got, tt.want)
			}
		})
	}
}

func TestDiffIsStableAcrossRuns(t *testing.T) {
	baseline := load(t, "testdata/old.json")
	current := load(t, "testdata/new.json")
	ops := []string{"SameOp", "ErrorOp", "AddMember", "GoneOp"}
	first, err := Diff(baseline, current, ops)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		again, err := Diff(baseline, current, ops)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(again, first) {
			t.Fatalf("re-run differs:\n%#v\n%#v", again, first)
		}
	}
	order := []string{first[0].Operation, first[1].Operation, first[2].Operation}
	if want := []string{"ErrorOp", "AddMember", "GoneOp"}; !reflect.DeepEqual(order, want) {
		t.Errorf("changes in order %v, want the order of ops %v", order, want)
	}
}

func TestDiffRejectsOperationMissingFromBaseline(t *testing.T) {
	baseline := load(t, "testdata/old.json")
	_, err := Diff(baseline, baseline, []string{"AddMember", "NoSuchOp"})
	var unknown *UnknownOperationError
	if !errors.As(err, &unknown) || unknown.Operation != "NoSuchOp" {
		t.Fatalf("Diff error = %v, want UnknownOperationError for NoSuchOp", err)
	}
}

func TestFactsFlattenListsMapsAndRecursion(t *testing.T) {
	got := load(t, "testdata/old.json").facts("SameOp")
	want := []string{
		"error NoSuchEntityException code=NoSuchEntity http=404",
		"input.Ids: list",
		"input.Ids[]: string",
		"input.Tags required",
		"input.Tags: map",
		"input.Tags{key}: string",
		"input.Tags{value}: string",
		"input: structure",
		"operation SameOp",
		"output.Children: list",
		"output.Children[]: recursive Node",
		"output: structure",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("facts(SameOp) =\n%q\nwant\n%q", got, want)
	}
}

func TestParseRejectsMalformedModels(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantRef *RefError
		wantErr error
	}{
		{name: "not JSON", model: `{"operations":`},
		{name: "no operations", model: `{"shapes":{}}`, wantErr: ErrNoOperations},
		{
			name:    "dangling operation input",
			model:   `{"operations":{"Op":{"input":{"shape":"Missing"}}},"shapes":{}}`,
			wantRef: &RefError{From: "operation Op input", Shape: "Missing"},
		},
		{
			name: "dangling structure member",
			model: `{"operations":{"Op":{}},"shapes":{"S":{"type":"structure",` +
				`"members":{"M":{"shape":"Missing"}}}}}`,
			wantRef: &RefError{From: "shape S member M", Shape: "Missing"},
		},
		{
			name:    "list without member",
			model:   `{"operations":{"Op":{}},"shapes":{"L":{"type":"list"}}}`,
			wantRef: &RefError{From: "shape L member"},
		},
		{
			name: "map without value",
			model: `{"operations":{"Op":{}},"shapes":{"K":{"type":"string"},` +
				`"M":{"type":"map","key":{"shape":"K"}}}}`,
			wantRef: &RefError{From: "shape M value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Parse([]byte(tt.model))
			if m != nil {
				t.Fatalf("Parse returned a model for a malformed input")
			}
			var syntax *json.SyntaxError
			var ref *RefError
			switch {
			case tt.wantRef != nil:
				if !errors.As(err, &ref) || *ref != *tt.wantRef {
					t.Errorf("Parse error = %v, want %v", err, tt.wantRef)
				}
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Parse error = %v, want %v", err, tt.wantErr)
				}
			case !errors.As(err, &syntax):
				t.Errorf("Parse error = %v, want a JSON syntax error", err)
			}
		})
	}
}
