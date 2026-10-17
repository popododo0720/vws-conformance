package sts

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssts "github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Caller() bind.Group {
	return bind.Group{Service: "sts", Name: "caller", Run: run, Cases: []cases.Case{
		{
			ID: "STS-GCI-001", Service: "sts", Group: "caller", Op: "GetCallerIdentity", Kind: cases.KindHappy,
			Suite: cases.SuiteSDK, DocURL: cases.DocSTS("GetCallerIdentity"), Expect: cases.Expect{
				HTTP:     200,
				Patterns: map[string]string{"arn": `^arn:aws:iam::[0-9]{12}:root$`},
			}, Impl: "yes",
		},
		{
			ID: "STS-GCI-101", Service: "sts", Group: "caller", Op: "GetCallerIdentity", Kind: cases.KindError,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors,
			Expect: cases.Expect{HTTP: 403, Code: "InvalidClientTokenId"},
			Skip:   cases.SkipIAM, Impl: "yes",
		},
	}}
}

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
			return wrapErr("run", err)
		})
		o.Fields["arn"] = arn
		return o, wrapErr("run", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}
