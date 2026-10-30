package iam_test

import (
	"testing"

	"github.com/popododo0720/vws-conformance/iam"
	"github.com/popododo0720/vws-conformance/internal/cases"
)

func TestMFAExpectations(t *testing.T) {
	const (
		docGetMFADevice = "https://docs.aws.amazon.com/IAM/latest/APIReference/API_GetMFADevice.html"
		docCommonErrors = "https://docs.aws.amazon.com/IAM/latest/APIReference/CommonErrors.html"
	)
	want := map[string]cases.Case{
		"IAM-MFA-001": {
			Op: "GetMFADevice", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: docGetMFADevice,
			Expect: cases.Expect{HTTP: 200},
		},
		"IAM-MFA-101": {
			Op: "GetMFADevice", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: docGetMFADevice,
			Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"},
		},
		"IAM-MFA-102": {
			Op: "GetMFADevice", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: docGetMFADevice,
			Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"},
		},
		"IAM-MFA-103": {
			Op: "GetMFADevice", Kind: cases.KindAuth, Suite: cases.SuiteSDK, DocURL: docCommonErrors,
			Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Skip: cases.SkipMFAAuth,
		},
	}
	g := iam.MFA()
	if g.Service != "iam" || g.Name != "mfa" {
		t.Fatalf("group = %s/%s, want iam/mfa", g.Service, g.Name)
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
		if got.Service != "iam" || got.Group != "mfa" {
			t.Errorf("%s service/group = %s/%s, want iam/mfa", got.ID, got.Service, got.Group)
		}
		if got.Op != w.Op || got.Kind != w.Kind || got.Suite != w.Suite || got.DocURL != w.DocURL {
			t.Errorf("%s = %s/%s/%s/%s, want %s/%s/%s/%s", got.ID, got.Op, got.Kind, got.Suite, got.DocURL,
				w.Op, w.Kind, w.Suite, w.DocURL)
		}
		if got.Expect.HTTP != w.Expect.HTTP || got.Expect.Code != w.Expect.Code {
			t.Errorf("%s expect = %d %q, want %d %q", got.ID, got.Expect.HTTP, got.Expect.Code,
				w.Expect.HTTP, w.Expect.Code)
		}
		if got.Skip != w.Skip {
			t.Errorf("%s skip = %q, want %q", got.ID, got.Skip, w.Skip)
		}
	}
	const serial = `^arn:aws:iam::[0-9]{12}:u2f/root/testFidoKey-R1234$`
	for _, got := range g.Cases {
		if got.ID == "IAM-MFA-001" && got.Expect.Patterns["SerialNumber"] != serial {
			t.Errorf("IAM-MFA-001 SerialNumber pattern = %q, want %q", got.Expect.Patterns["SerialNumber"], serial)
		}
	}
}
