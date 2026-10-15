package sts

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Caller() bind.Group {
	skip := cases.SkipIAM
	return bind.Group{Service: "sts", Name: "caller", Run: run, Cases: []cases.Case{
		{ID: "STS-GCI-001", Service: "sts", Group: "caller", Op: "GetCallerIdentity", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocSTS("GetCallerIdentity"), Expect: cases.Expect{HTTP: 200, Patterns: map[string]string{"arn": `^arn:aws:iam::[0-9]{12}:root$`}}, Skip: skip, Impl: "yes"},
		{ID: "STS-GCI-101", Service: "sts", Group: "caller", Op: "GetCallerIdentity", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors, Expect: cases.Expect{HTTP: 403, Code: "InvalidClientTokenId"}, Skip: skip, Impl: "yes"},
		{ID: "STS-AR-001", Service: "sts", Group: "caller", Op: "AssumeRole", Kind: cases.KindHappy, Suite: cases.SuiteSDK, DocURL: cases.DocSTS("AssumeRole"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "yes"},
		{ID: "STS-AR-101", Service: "sts", Group: "caller", Op: "AssumeRole", Kind: cases.KindError, Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors, Expect: cases.Expect{HTTP: 400, Code: "ValidationError"}, Skip: skip, Impl: "yes"},
		{ID: "STS-GAP-001", Service: "sts", Group: "caller", Op: "GetSessionToken", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocSTS("GetSessionToken"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "STS-GAP-002", Service: "sts", Group: "caller", Op: "AssumeRoleWithWebIdentity", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: cases.DocSTS("AssumeRoleWithWebIdentity"), Expect: cases.Expect{HTTP: 400, Code: "InvalidIdentityToken"}, Skip: skip, Impl: "gap"},
	}}
}

func Cases() []cases.Case { return Caller().Cases }

func run(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.STS == nil {
		return cases.Outcome{}, fmt.Errorf("sts client is unset")
	}
	api := rt.Clients.STS
	switch c.ID {
	case "STS-GCI-001":
		var arn string
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			out, err := api.GetCallerIdentity(ctx, &awssts.GetCallerIdentityInput{})
			if err == nil {
				arn = aws.ToString(out.Arn)
			}
			return err
		})
		o.Fields["arn"] = arn
		return o, err
	case "STS-AR-101":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.AssumeRole(ctx, &awssts.AssumeRoleInput{RoleArn: aws.String("arn:aws:iam::123456789012:role/conf"), RoleSessionName: aws.String("s"), DurationSeconds: aws.Int32(60)})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("sts case %s is skipped until vws-deploy#310", c.ID)
	}
}

func Skipped() bool {
	for _, c := range Cases() {
		if !strings.Contains(c.Skip, "vws-deploy#310") {
			return false
		}
	}
	return true
}
