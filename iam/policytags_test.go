package iam_test

import (
	"slices"
	"testing"

	"github.com/popododo0720/vws-conformance/iam"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestPolicyTagsExpectations(t *testing.T) {
	want := map[string]cases.Expect{
		"IAM-PTAG-101": {HTTP: 404, Code: "NoSuchEntity"},
		"IAM-PTAG-102": {HTTP: 400, Code: "InvalidInput"},
	}
	g := iam.PolicyTags()
	g.Cases = slices.DeleteFunc(slices.Clone(g.Cases), func(c cases.Case) bool { return c.Op != "ListPolicyTags" })
	if len(g.Cases) != len(want) {
		t.Fatalf("cases = %d, want %d", len(g.Cases), len(want))
	}
	for _, got := range g.Cases {
		w, ok := want[got.ID]
		if !ok {
			t.Fatalf("unexpected case %s", got.ID)
		}
		if got.Op != "ListPolicyTags" || got.Kind != cases.KindError || got.DocURL != cases.DocIAM("ListPolicyTags") {
			t.Errorf("%s = %s/%s/%s, want ListPolicyTags/error/%s", got.ID, got.Op, got.Kind, got.DocURL,
				cases.DocIAM("ListPolicyTags"))
		}
		if got.Expect.HTTP != w.HTTP || got.Expect.Code != w.Code {
			t.Errorf("%s expect = %d %q, want %d %q", got.ID, got.Expect.HTTP, got.Expect.Code, w.HTTP, w.Code)
		}
		if got.Skip != "" {
			t.Errorf("%s skip = %q, want runnable", got.ID, got.Skip)
		}
	}
}
