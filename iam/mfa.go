package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func MFA() bind.Group {
	return bind.Group{Service: "iam", Name: "mfa", Run: runMFA, Cases: []cases.Case{
		{
			ID: "IAM-MFA-001", Service: "iam", Group: "mfa", Op: "GetMFADevice", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("GetMFADevice"), Expect: cases.Expect{
				HTTP:     200,
				Patterns: map[string]string{"SerialNumber": `^arn:aws:iam::[0-9]{12}:u2f/root/testFidoKey-R1234$`},
			}, Impl: "yes",
		},
		{
			ID: "IAM-MFA-101", Service: "iam", Group: "mfa", Op: "GetMFADevice", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("GetMFADevice"),
			Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"}, Impl: "yes",
		},
		{
			ID: "IAM-MFA-102", Service: "iam", Group: "mfa", Op: "GetMFADevice", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAM("GetMFADevice"),
			Expect: cases.Expect{HTTP: 404, Code: "NoSuchEntity"}, Impl: "yes",
		},
		{
			// The suite runs one credential, so the same caller cannot also lack
			// iam:GetMFADevice. The skip waits for a scoped credential
			// (popododo0720/vws-deploy#310).
			ID: "IAM-MFA-103", Service: "iam", Group: "mfa", Op: "GetMFADevice", Kind: cases.KindAuth,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors,
			Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Skip: cases.SkipMFAAuth, Impl: "yes",
		},
	}}
}

func runMFA(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	if rt.Cfg.AccountID == "" {
		return cases.Outcome{}, fmt.Errorf("account ID is unset")
	}
	var serial string
	switch c.ID {
	case "IAM-MFA-001":
		serial = fmt.Sprintf("arn:aws:iam::%s:u2f/root/testFidoKey-R1234", rt.Cfg.AccountID)
	case "IAM-MFA-101":
		serial = fmt.Sprintf("arn:aws:iam::%s:mfa/%s", rt.Cfg.AccountID, rt.Name("mfa"))
	case "IAM-MFA-102":
		serial = fmt.Sprintf("arn:aws:iam::%s:u2f/root/%s", rt.Cfg.AccountID, rt.Name("fido"))
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
	var out *awsiam.GetMFADeviceOutput
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		var callErr error
		out, callErr = rt.Clients.IAM.GetMFADevice(ctx, &awsiam.GetMFADeviceInput{
			SerialNumber: aws.String(serial),
		})
		return wrapErr("runMFA", callErr)
	})
	if err != nil {
		return o, wrapErr("runMFA", err)
	}
	if out != nil {
		o.Fields["SerialNumber"] = aws.ToString(out.SerialNumber)
		o.Fields["UserName"] = aws.ToString(out.UserName)
		if out.EnableDate != nil {
			o.Fields["EnableDate"] = out.EnableDate.Format(time.RFC3339)
		}
	}
	return o, nil
}
