package iam

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func Users() bind.Group {
	skip := cases.SkipIAM
	doc := cases.DocIAM
	errDoc := cases.DocIAMErrors
	c := func(id, op, kind string, exp cases.Expect) cases.Case {
		return cases.Case{ID: id, Service: "iam", Group: "users", Op: op, Kind: kind, Suite: cases.SuiteSDK, DocURL: doc(op), Expect: exp, Skip: skip, Impl: "yes"}
	}
	g := bind.Group{Service: "iam", Name: "users", Run: runUsers, Cases: []cases.Case{
		c("IAM-USR-001", "CreateUser", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-USR-002", "GetUser", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-USR-003", "ListUsers", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-USR-004", "DeleteUser", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-USR-101", "CreateUser", cases.KindError, cases.Expect{HTTP: 409, Code: "EntityAlreadyExists"}),
		c("IAM-USR-102", "GetUser", cases.KindError, cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		c("IAM-USR-103", "DeleteUser", cases.KindError, cases.Expect{HTTP: 409, Code: "DeleteConflict"}),
		c("IAM-USR-104", "CreateUser", cases.KindError, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		c("IAM-AK-001", "CreateAccessKey", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-AK-101", "CreateAccessKey", cases.KindError, cases.Expect{HTTP: 409, Code: "LimitExceeded"}),
		c("IAM-GRP-001", "CreateGroup", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-GRP-101", "GetGroup", cases.KindError, cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		c("IAM-ROLE-001", "CreateRole", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-ROLE-101", "CreateRole", cases.KindError, cases.Expect{HTTP: 400, Code: "MalformedPolicyDocument"}),
		c("IAM-POL-001", "CreatePolicy", cases.KindHappy, cases.Expect{HTTP: 200}),
		c("IAM-POL-101", "CreatePolicy", cases.KindError, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		{ID: "IAM-GAP-001", Service: "iam", Group: "users", Op: "CreatePolicyVersion", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: doc("CreatePolicyVersion"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "IAM-GAP-002", Service: "iam", Group: "users", Op: "ListPolicyVersions", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: doc("ListPolicyVersions"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "IAM-GAP-003", Service: "iam", Group: "users", Op: "DeletePolicyVersion", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: doc("DeletePolicyVersion"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "IAM-GAP-004", Service: "iam", Group: "users", Op: "SetDefaultPolicyVersion", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: errDoc, Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "IAM-GAP-005", Service: "iam", Group: "users", Op: "CreateVirtualMFADevice", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: doc("CreateVirtualMFADevice"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
		{ID: "IAM-GAP-006", Service: "iam", Group: "users", Op: "UpdateAccessKey", Kind: cases.KindGap, Suite: cases.SuiteSDK, DocURL: doc("UpdateAccessKey"), Expect: cases.Expect{HTTP: 200}, Skip: skip, Impl: "gap"},
	}}
	g.Cases = fillIAM(g.Cases, c)
	return g
}

func fillIAM(in []cases.Case, c func(id, op, kind string, exp cases.Expect) cases.Case) []cases.Case {
	happy := map[string]bool{}
	bad := map[string]bool{}
	for _, item := range in {
		if item.Impl != "yes" {
			continue
		}
		if item.Kind == cases.KindError {
			bad[item.Op] = true
			continue
		}
		happy[item.Op] = true
	}
	for _, op := range iamImplemented {
		if !happy[op] {
			in = append(in, c("IAM-H-"+op, op, cases.KindHappy, cases.Expect{HTTP: 200}))
		}
		if !bad[op] {
			code, status := iamError(op)
			in = append(in, c("IAM-E-"+op, op, cases.KindError, cases.Expect{HTTP: status, Code: code}))
		}
	}
	return in
}

func iamError(op string) (string, int) {
	switch {
	case strings.HasPrefix(op, "List"):
		return "ValidationError", 400
	case strings.HasPrefix(op, "Get"), strings.HasPrefix(op, "Delete"), strings.HasPrefix(op, "Detach"), strings.HasPrefix(op, "Remove"):
		return "NoSuchEntity", 404
	default:
		return "ValidationError", 400
	}
}

var iamImplemented = []string{
	"CreateUser", "GetUser", "ListUsers", "DeleteUser",
	"CreateLoginProfile", "GetLoginProfile", "UpdateLoginProfile", "DeleteLoginProfile",
	"CreateAccessKey", "ListAccessKeys", "DeleteAccessKey",
	"CreateGroup", "GetGroup", "ListGroups", "DeleteGroup",
	"AddUserToGroup", "RemoveUserFromGroup", "ListGroupsForUser",
	"CreateRole", "GetRole", "ListRoles", "DeleteRole",
	"CreatePolicy", "GetPolicy", "GetPolicyVersion", "ListPolicies", "DeletePolicy",
	"AttachUserPolicy", "DetachUserPolicy", "ListAttachedUserPolicies", "PutUserPolicy", "GetUserPolicy", "DeleteUserPolicy", "ListUserPolicies",
	"AttachGroupPolicy", "DetachGroupPolicy", "ListAttachedGroupPolicies", "PutGroupPolicy", "GetGroupPolicy", "DeleteGroupPolicy", "ListGroupPolicies",
	"AttachRolePolicy", "DetachRolePolicy", "ListAttachedRolePolicies", "PutRolePolicy", "GetRolePolicy", "DeleteRolePolicy", "ListRolePolicies",
}

func Cases() []cases.Case { return Users().Cases }

func runUsers(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	api := rt.Clients.IAM
	name := rt.Name("user")
	path := "/conformance/"
	switch c.ID {
	case "IAM-USR-001":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name), Path: aws.String(path)})
			return err
		})
	case "IAM-USR-002":
		if _, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name), Path: aws.String(path)}); err != nil {
			return cases.Outcome{}, err
		}
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.GetUser(ctx, &awsiam.GetUserInput{UserName: aws.String(name)})
			return err
		})
	case "IAM-USR-003":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ListUsers(ctx, &awsiam.ListUsersInput{PathPrefix: aws.String(path)})
			return err
		})
	case "IAM-USR-004":
		if _, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name), Path: aws.String(path)}); err != nil {
			return cases.Outcome{}, err
		}
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.DeleteUser(ctx, &awsiam.DeleteUserInput{UserName: aws.String(name)})
			return err
		})
	case "IAM-USR-101":
		if _, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name), Path: aws.String(path)}); err != nil {
			return cases.Outcome{}, err
		}
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(name), Path: aws.String(path)})
			return err
		})
	case "IAM-USR-102":
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.GetUser(ctx, &awsiam.GetUserInput{UserName: aws.String(rt.Name("missing"))})
			return err
		})
	case "IAM-USR-104":
		long := rt.Name("user") + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
		return rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateUser(ctx, &awsiam.CreateUserInput{UserName: aws.String(long)})
			return err
		})
	default:
		return cases.Outcome{}, fmt.Errorf("iam case %s is skipped until vws-deploy#310", c.ID)
	}
}
