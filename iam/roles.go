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

// No principal can assume a role with this trust policy.
const (
	trustPolicy = `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Deny","Principal":{"AWS":"*"},"Action":"sts:AssumeRole"}]}`
	malformedTrustPolicy = `{"Version":"2012-10-17","Statement":`
)

func Roles() bind.Group {
	return bind.Group{Service: "iam", Name: "roles", Run: runRoles, Cases: []cases.Case{
		roleCase("IAM-ROLE-001", "CreateRole", cases.KindHappy, cases.Expect{
			HTTP: 200, Patterns: map[string]string{"role": expect.ShapeOKPattern},
		}),
		roleCase("IAM-ROLE-101", "CreateRole", cases.KindError,
			cases.Expect{HTTP: 400, Code: "MalformedPolicyDocument"}),
		roleCase("IAM-H-DeleteRole", "DeleteRole", cases.KindHappy, cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "after": `^NoSuchEntity$`},
		}),
		roleCase("IAM-E-DeleteRole", "DeleteRole", cases.KindError, cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
	}}
}

func roleCase(id, op, kind string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "roles", Op: op, Kind: kind, Suite: cases.SuiteSDK,
		DocURL: cases.DocIAM(op), Expect: exp, Impl: "yes",
	}
}

func runRoles(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, errors.New("iam client is unset")
	}
	switch c.ID {
	case "IAM-ROLE-001":
		return createRoleCase(ctx, rt, rt.Name("role-001"))
	case "IAM-ROLE-101":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMCreateRole(ctx, &awsiam.CreateRoleInput{
				RoleName: aws.String(rt.Name("role-101")), AssumeRolePolicyDocument: aws.String(malformedTrustPolicy),
			})
			return wrapErr("IAMCreateRole", err)
		})
		return o, wrapErr("runRoles", err)
	case "IAM-H-DeleteRole":
		return deleteRoleCase(ctx, rt, rt.Name("role-hdel"))
	case "IAM-E-DeleteRole":
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := rt.IAMProbeDeleteRole(ctx, rt.Name("role-missing"))
			return wrapErr("IAMProbeDeleteRole", err)
		})
		return o, wrapErr("runRoles", err)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

// createRoleCase measures CreateRole and checks the returned role names what was asked for. The
// gate deletes the role when the case ends.
func createRoleCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	var out *awsiam.CreateRoleOutput
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		var callErr error
		out, callErr = rt.IAMCreateRole(ctx, &awsiam.CreateRoleInput{
			RoleName: aws.String(name), AssumeRolePolicyDocument: aws.String(trustPolicy),
		})
		return wrapErr("IAMCreateRole", callErr)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("createRoleCase", err)
	}
	o.Fields["role"] = roleShape(out, name)
	return o, nil
}

// roleShape checks the Role members the CreateRole reference marks as always returned.
func roleShape(out *awsiam.CreateRoleOutput, name string) string {
	switch {
	case out == nil || out.Role == nil:
		return "response has no Role"
	case aws.ToString(out.Role.RoleName) != name:
		return fmt.Sprintf("RoleName %q, want %q", aws.ToString(out.Role.RoleName), name)
	case !strings.HasSuffix(aws.ToString(out.Role.Arn), ":role/"+name):
		return fmt.Sprintf("Arn %q does not name role/%s", aws.ToString(out.Role.Arn), name)
	case aws.ToString(out.Role.RoleId) == "":
		return "RoleId is empty"
	case out.Role.CreateDate == nil:
		return "CreateDate is missing"
	}
	return expect.ShapeOK
}

// deleteRoleCase creates a run-owned role, measures DeleteRole on it and records the error code a
// GetRole answers afterwards.
func deleteRoleCase(ctx context.Context, rt *harness.Runtime, name string) (cases.Outcome, error) {
	if err := setup(ctx, func(ctx context.Context) error {
		_, err := rt.IAMCreateRole(ctx, &awsiam.CreateRoleInput{
			RoleName: aws.String(name), AssumeRolePolicyDocument: aws.String(trustPolicy),
		})
		return wrapErr("IAMCreateRole", err)
	}); err != nil {
		return cases.Outcome{}, wrapErr("deleteRoleCase", err)
	}
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := rt.IAMDeleteRole(ctx, name)
		return wrapErr("IAMDeleteRole", err)
	})
	if err != nil || o.Code != "" {
		return o, wrapErr("deleteRoleCase", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "DeleteRole")
	getErr := setup(ctx, func(ctx context.Context) error {
		_, err := rt.Clients.IAM.GetRole(ctx, &awsiam.GetRoleInput{RoleName: aws.String(name)})
		return wrapErr("GetRole", err)
	})
	var api smithy.APIError
	switch {
	case getErr == nil:
		o.Fields["after"] = "role still present"
	case errors.As(getErr, &api):
		o.Fields["after"] = api.ErrorCode()
	default:
		return o, wrapErr("deleteRoleCase", getErr)
	}
	return o, nil
}
