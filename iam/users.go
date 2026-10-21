package iam

import (
	"context"
	"fmt"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Users() bind.Group {
	return bind.Group{Service: "iam", Name: "users", Run: runUsers, Cases: []cases.Case{
		iamRead("IAM-USR-003", "ListUsers", cases.KindHappy, cases.Expect{HTTP: 200}),
		iamRead("IAM-USR-102", "GetUser", cases.KindError, cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		iamRead("IAM-GRP-101", "GetGroup", cases.KindError, cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		{
			ID: "IAM-GAP-002", Service: "iam", Group: "users", Op: "ListPolicyVersions", Kind: cases.KindGap,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("ListPolicyVersions"), Expect: cases.Expect{HTTP: 200},
			Skip: cases.SkipIAM, Impl: "gap",
		},
	}}
}

func iamRead(id, op, kind string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "users", Op: op, Kind: kind, Suite: cases.SuiteSDK,
		DocURL: cases.DocIAM(op), Expect: exp, Skip: cases.SkipIAM, Impl: "yes",
	}
}

func runUsers(_ context.Context, _ *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	return cases.Outcome{}, fmt.Errorf("iam case %s is skipped until vws-deploy#310", c.ID)
}
