package iam_test

import (
	"testing"

	"github.com/popododo0720/vws-conformance/iam"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestPolicyTagsExpectations(t *testing.T) {
	const (
		docUntag   = "https://docs.aws.amazon.com/IAM/latest/APIReference/API_UntagPolicy.html"
		docErrors  = "https://docs.aws.amazon.com/IAM/latest/APIReference/CommonErrors.html"
		docCrossAc = "https://docs.aws.amazon.com/IAM/latest/UserGuide/" +
			"reference_policies_evaluation-logic-cross-account.html"
	)
	validation := cases.Case{
		Kind: cases.KindError, DocURL: docErrors, Expect: cases.Expect{HTTP: 400, Code: "ValidationError"},
	}
	want := map[string]cases.Case{
		"IAM-PTAG-001": {Kind: cases.KindTags, DocURL: docUntag, Expect: cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": "^ok$", "tags": `^conf-c=3$`},
		}},
		"IAM-PTAG-002": {Kind: cases.KindIdempotency, DocURL: docUntag, Expect: cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": "^ok$", "tags": `^conf-a=1$`},
		}},
		"IAM-PTAG-101": validation,
		"IAM-PTAG-102": validation,
		"IAM-PTAG-103": validation,
		"IAM-PTAG-104": validation,
		"IAM-PTAG-105": {Kind: cases.KindError, DocURL: docUntag, Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"}},
		"IAM-PTAG-106": {Kind: cases.KindAuth, DocURL: docCrossAc, Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}},
		"IAM-PTAG-107": {
			Kind: cases.KindAuth, DocURL: docErrors, Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"},
			Skip: cases.SkipUntagPolicyAuth,
		},
	}
	g := iam.PolicyTags()
	if g.Service != "iam" || g.Name != "policytags" {
		t.Fatalf("group = %s/%s, want iam/policytags", g.Service, g.Name)
	}
	if len(g.Cases) != len(want) {
		t.Fatalf("cases = %d, want %d", len(g.Cases), len(want))
	}
	seen := map[string]bool{}
	for _, got := range g.Cases {
		if seen[got.ID] {
			t.Fatalf("case %s appears twice", got.ID)
		}
		seen[got.ID] = true
		w, ok := want[got.ID]
		if !ok {
			t.Fatalf("unexpected case %s", got.ID)
		}
		if got.Service != "iam" || got.Group != "policytags" || got.Op != "UntagPolicy" || got.Suite != cases.SuiteSDK {
			t.Errorf("%s = %s/%s/%s/%s, want iam/policytags/UntagPolicy/%s",
				got.ID, got.Service, got.Group, got.Op, got.Suite, cases.SuiteSDK)
		}
		if got.Kind != w.Kind || got.DocURL != w.DocURL || got.Skip != w.Skip {
			t.Errorf("%s kind/doc/skip = %s/%s/%q, want %s/%s/%q",
				got.ID, got.Kind, got.DocURL, got.Skip, w.Kind, w.DocURL, w.Skip)
		}
		if got.Expect.HTTP != w.Expect.HTTP || got.Expect.Code != w.Expect.Code {
			t.Errorf("%s expect = %d %q, want %d %q", got.ID, got.Expect.HTTP, got.Expect.Code,
				w.Expect.HTTP, w.Expect.Code)
		}
		if len(got.Expect.Patterns) != len(w.Expect.Patterns) {
			t.Errorf("%s patterns = %v, want %v", got.ID, got.Expect.Patterns, w.Expect.Patterns)
		}
		for field, pat := range w.Expect.Patterns {
			if got.Expect.Patterns[field] != pat {
				t.Errorf("%s pattern %s = %q, want %q", got.ID, field, got.Expect.Patterns[field], pat)
			}
		}
	}
}
