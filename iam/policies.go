package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/smithy-go"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// A policy name allows only [\w+=,.@-] (CreatePolicy reference), so a space and "!" are invalid.
const invalidNameSuffix = " bad!"

func Policies() bind.Group {
	return bind.Group{Service: "iam", Name: "policies", Run: runPolicies, Cases: []cases.Case{
		policyCase("IAM-POL-001", "CreatePolicy", cases.KindHappy, cases.DocIAM("CreatePolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"policy": expect.ShapeOKPattern},
		}),
		policyCase("IAM-POL-101", "CreatePolicy", cases.KindError, cases.DocIAMErrors,
			cases.Expect{HTTP: 400, Code: "ValidationError"}),
		policyCase("IAM-GAP-001", "CreatePolicyVersion", cases.KindGap, cases.DocIAM("CreatePolicyVersion"),
			cases.Expect{HTTP: 200, Patterns: map[string]string{"version": `^v2 default=false$`}}),
		policyCase("IAM-GAP-003", "DeletePolicyVersion", cases.KindGap, cases.DocIAM("DeletePolicyVersion"),
			cases.Expect{HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "versions": `^v1$`}}),
		policyCase("IAM-GAP-004", "SetDefaultPolicyVersion", cases.KindGap, cases.DocIAM("SetDefaultPolicyVersion"),
			cases.Expect{HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "default": `^v2$`}}),
		policyCase("IAM-H-DeletePolicy", "DeletePolicy", cases.KindHappy, cases.DocIAM("DeletePolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "after": `^NoSuchEntity$`},
		}),
		policyCase("IAM-E-DeletePolicy", "DeletePolicy", cases.KindError, cases.DocIAM("DeletePolicy"),
			cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
	}}
}

func policyCase(id, op, kind, doc string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "policies", Op: op, Kind: kind, Suite: cases.SuiteSDK,
		DocURL: doc, Expect: exp, Impl: "yes",
	}
}

func runPolicies(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, errors.New("iam client is unset")
	}
	switch c.ID {
	case "IAM-POL-001":
		return createPolicyCase(ctx, rt, rt.Name("pol-001"))
	case "IAM-POL-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMCreatePolicy(ctx, &awsiam.CreatePolicyInput{
				PolicyName: aws.String(rt.Name("pol-101") + invalidNameSuffix), PolicyDocument: aws.String(policyDocument),
			})
			return wrapErr("IAMCreatePolicy", err)
		})
		return o, wrapErr("runPolicies", err)
	case "IAM-GAP-001":
		return createVersionCase(ctx, rt, rt.Name("pol-gap001"))
	case "IAM-GAP-003":
		return deleteVersionCase(ctx, rt, rt.Name("pol-gap003"))
	case "IAM-GAP-004":
		return setDefaultCase(ctx, rt, rt.Name("pol-gap004"))
	case "IAM-H-DeletePolicy":
		return deletePolicyCase(ctx, rt, rt.Name("pol-hdel"))
	case "IAM-E-DeletePolicy":
		if rt.Cfg.AccountID == "" {
			return cases.Outcome{}, errors.New("account ID is unset")
		}
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMProbeDeletePolicy(ctx, policyARN(rt.Cfg.AccountID, rt.Name("pol-missing")))
			return wrapErr("IAMProbeDeletePolicy", err)
		})
		return o, wrapErr("runPolicies", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

// createPolicyCase measures CreatePolicy and checks the returned policy. The gate deletes it when
// the case ends.
func createPolicyCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	var out *awsiam.CreatePolicyOutput
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		var callErr error
		out, callErr = rt.IAMCreatePolicy(ctx, &awsiam.CreatePolicyInput{
			PolicyName: aws.String(name), PolicyDocument: aws.String(policyDocument),
		})
		return wrapErr("IAMCreatePolicy", callErr)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("createPolicyCase", err)
	}
	o.Fields["policy"] = policyShape(out, name)
	return o, nil
}

// policyShape checks the Policy members a new policy has: its name, ARN, id, creation date and v1
// as the default version (CreatePolicy reference).
func policyShape(out *awsiam.CreatePolicyOutput, name string) string {
	switch {
	case out == nil || out.Policy == nil:
		return "response has no Policy"
	case aws.ToString(out.Policy.PolicyName) != name:
		return fmt.Sprintf("PolicyName %q, want %q", aws.ToString(out.Policy.PolicyName), name)
	case !strings.HasSuffix(aws.ToString(out.Policy.Arn), ":policy/"+name):
		return fmt.Sprintf("Arn %q does not name policy/%s", aws.ToString(out.Policy.Arn), name)
	case aws.ToString(out.Policy.PolicyId) == "":
		return "PolicyId is empty"
	case aws.ToString(out.Policy.DefaultVersionId) != "v1":
		return fmt.Sprintf("DefaultVersionId %q, want v1", aws.ToString(out.Policy.DefaultVersionId))
	case out.Policy.CreateDate == nil:
		return "CreateDate is missing"
	}
	return expect.ShapeOK
}

// createVersionCase measures CreatePolicyVersion on a new owned policy and records the returned
// version id and default flag.
func createVersionCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	arn, err := createPolicy(ctx, rt, name)
	if err != nil {
		return cases.Outcome{}, wrapErr("createVersionCase", err)
	}
	var out *awsiam.CreatePolicyVersionOutput
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		var callErr error
		out, callErr = rt.IAMCreatePolicyVersion(ctx, &awsiam.CreatePolicyVersionInput{
			PolicyArn: aws.String(arn), PolicyDocument: aws.String(policyDocument),
		})
		return wrapErr("IAMCreatePolicyVersion", callErr)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("createVersionCase", err)
	}
	if out.PolicyVersion == nil {
		o.Fields["version"] = "response has no PolicyVersion"
		return o, nil
	}
	o.Fields["version"] = fmt.Sprintf("%s default=%t",
		aws.ToString(out.PolicyVersion.VersionId), out.PolicyVersion.IsDefaultVersion)
	return o, nil
}

// deleteVersionCase adds v2 to an owned policy, measures DeletePolicyVersion on it and records the
// versions left.
func deleteVersionCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	arn, version, err := policyWithVersion(ctx, rt, name)
	if err != nil {
		return cases.Outcome{}, wrapErr("deleteVersionCase", err)
	}
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := rt.IAMDeletePolicyVersion(ctx, &awsiam.DeletePolicyVersionInput{
			PolicyArn: aws.String(arn), VersionId: aws.String(version),
		})
		return wrapErr("IAMDeletePolicyVersion", err)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("deleteVersionCase", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "DeletePolicyVersion")
	var left *awsiam.ListPolicyVersionsOutput
	if err := setup(ctx, func(ctx context.Context) error {
		var err error
		left, err = rt.Clients.IAM.ListPolicyVersions(ctx, &awsiam.ListPolicyVersionsInput{PolicyArn: aws.String(arn)})
		return wrapErr("ListPolicyVersions", err)
	}); err != nil {
		return o, wrapErr("deleteVersionCase", err)
	}
	ids := make([]string, len(left.Versions))
	for i, v := range left.Versions {
		ids[i] = aws.ToString(v.VersionId)
	}
	o.Fields["versions"] = strings.Join(ids, ",")
	return o, nil
}

// setDefaultCase adds v2 to an owned policy, measures SetDefaultPolicyVersion to it and records the
// default GetPolicy reports. Cleanup then has to delete v1 before the policy.
func setDefaultCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	arn, version, err := policyWithVersion(ctx, rt, name)
	if err != nil {
		return cases.Outcome{}, wrapErr("setDefaultCase", err)
	}
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := rt.IAMSetDefaultPolicyVersion(ctx, &awsiam.SetDefaultPolicyVersionInput{
			PolicyArn: aws.String(arn), VersionId: aws.String(version),
		})
		return wrapErr("IAMSetDefaultPolicyVersion", err)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("setDefaultCase", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "SetDefaultPolicyVersion")
	var got *awsiam.GetPolicyOutput
	if err := setup(ctx, func(ctx context.Context) error {
		var err error
		got, err = rt.Clients.IAM.GetPolicy(ctx, &awsiam.GetPolicyInput{PolicyArn: aws.String(arn)})
		return wrapErr("GetPolicy", err)
	}); err != nil {
		return o, wrapErr("setDefaultCase", err)
	}
	if got.Policy == nil {
		o.Fields["default"] = "GetPolicy returned no Policy"
		return o, nil
	}
	o.Fields["default"] = aws.ToString(got.Policy.DefaultVersionId)
	return o, nil
}

// policyWithVersion creates an owned policy with a second, non-default version and returns the
// policy ARN and that version's id.
func policyWithVersion(ctx context.Context, rt *harness.Runtime, name string) (string, string, error) {
	arn, err := createPolicy(ctx, rt, name)
	if err != nil {
		return "", "", wrapErr("policyWithVersion", err)
	}
	var version string
	err = setup(ctx, func(ctx context.Context) error {
		out, err := rt.IAMCreatePolicyVersion(ctx, &awsiam.CreatePolicyVersionInput{
			PolicyArn: aws.String(arn), PolicyDocument: aws.String(policyDocument),
		})
		if err != nil {
			return wrapErr("IAMCreatePolicyVersion", err)
		}
		if out.PolicyVersion == nil {
			return errors.New("CreatePolicyVersion returned no PolicyVersion")
		}
		version = aws.ToString(out.PolicyVersion.VersionId)
		return nil
	})
	return arn, version, wrapErr("policyWithVersion", err)
}

// deletePolicyCase creates an owned policy, measures DeletePolicy on it and records the error code
// a GetPolicy answers afterwards.
func deletePolicyCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	arn, err := createPolicy(ctx, rt, name)
	if err != nil {
		return cases.Outcome{}, wrapErr("deletePolicyCase", err)
	}
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := rt.IAMDeletePolicy(ctx, arn)
		return wrapErr("IAMDeletePolicy", err)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("deletePolicyCase", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "DeletePolicy")
	getErr := setup(ctx, func(ctx context.Context) error {
		_, err := rt.Clients.IAM.GetPolicy(ctx, &awsiam.GetPolicyInput{PolicyArn: aws.String(arn)})
		return wrapErr("GetPolicy", err)
	})
	var api smithy.APIError
	switch {
	case getErr == nil:
		o.Fields["after"] = "policy still present"
	case errors.As(getErr, &api):
		o.Fields["after"] = api.ErrorCode()
	default:
		return o, wrapErr("deletePolicyCase", getErr)
	}
	return o, nil
}
