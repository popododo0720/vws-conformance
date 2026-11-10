package iam

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

const malformedPolicyARN = "not-a-policy-arn-value"

func PolicyTags() bind.Group {
	return bind.Group{Service: "iam", Name: "policytags", Run: runPolicyTags, Cases: []cases.Case{
		{
			ID: "IAM-PTAG-101", Service: "iam", Group: "policytags", Op: "ListPolicyTags", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("ListPolicyTags"),
			Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"}, Impl: "yes",
		},
		{
			ID: "IAM-PTAG-102", Service: "iam", Group: "policytags", Op: "ListPolicyTags", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("ListPolicyTags"),
			Expect: cases.Expect{HTTP: 400, Code: "InvalidInput"}, Impl: "yes",
		},
	}}
}

func runPolicyTags(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	if rt.Cfg.AccountID == "" {
		return cases.Outcome{}, fmt.Errorf("account ID is unset")
	}
	var arn string
	switch c.ID {
	case "IAM-PTAG-101":
		arn = fmt.Sprintf("arn:aws:iam::%s:policy/%s", rt.Cfg.AccountID, rt.Name("ptag"))
	case "IAM-PTAG-102":
		arn = malformedPolicyARN
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, callErr := rt.Clients.IAM.ListPolicyTags(ctx, &awsiam.ListPolicyTagsInput{PolicyArn: aws.String(arn)})
		return wrapErr("runPolicyTags", callErr)
	})
	if err != nil {
		return o, wrapErr("runPolicyTags", err)
	}
	return o, nil
}
