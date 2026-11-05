package iam

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

// The policy is never attached; Deny keeps it harmless if it ever were.
const policyDocument = `{"Version":"2012-10-17","Statement":[` +
	`{"Effect":"Deny","Action":"iam:ListPolicyTags","Resource":"*"}]}`

// UntagPolicy limits from the API reference: at most 50 TagKeys, each 1 to 128 characters.
const (
	maxTagKeys   = 50
	maxTagKeyLen = 128
)

func PolicyTags() bind.Group {
	return bind.Group{Service: "iam", Name: "policytags", Run: runPolicyTags, Cases: []cases.Case{
		untagCase("IAM-PTAG-001", cases.KindTags, cases.DocIAM("UntagPolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "tags": `^conf-c=3$`},
		}),
		untagCase("IAM-PTAG-002", cases.KindIdempotency, cases.DocIAM("UntagPolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "tags": `^conf-a=1$`},
		}),
		untagCase("IAM-PTAG-101", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-102", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-103", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-104", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-105", cases.KindError, cases.DocIAM("UntagPolicy"),
			cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		untagCase("IAM-PTAG-106", cases.KindAuth, cases.DocIAMCrossAccount,
			cases.Expect{HTTP: 403, Code: "AccessDenied"}),
		{
			ID: "IAM-PTAG-107", Service: "iam", Group: "policytags", Op: "UntagPolicy", Kind: cases.KindAuth,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors,
			Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Skip: cases.SkipUntagPolicyAuth, Impl: "yes",
		},
	}}
}

func untagCase(id, kind, doc string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "policytags", Op: "UntagPolicy", Kind: kind,
		Suite: cases.SuiteSDK, DocURL: doc, Expect: exp, Impl: "yes",
	}
}

func runPolicyTags(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	if rt.Clients == nil || rt.Clients.IAM == nil {
		return cases.Outcome{}, fmt.Errorf("iam client is unset")
	}
	if rt.Cfg.AccountID == "" {
		return cases.Outcome{}, fmt.Errorf("account ID is unset")
	}
	missing := aws.String(policyARN(rt.Cfg.AccountID, rt.Name("ptag-missing")))
	switch c.ID {
	case "IAM-PTAG-001":
		return untagOwned(ctx, rt, "ptag-001", nil, []string{"conf-a", "conf-b"},
			tag("conf-a", "1"), tag("conf-b", "2"), tag("conf-c", "3"))
	case "IAM-PTAG-002":
		return untagOwned(ctx, rt, "ptag-002", []string{"conf-b"}, []string{"conf-b", "conf-absent"},
			tag("conf-a", "1"), tag("conf-b", "2"))
	case "IAM-PTAG-101":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{TagKeys: []string{"conf-a"}}, withoutClientValidation)
	case "IAM-PTAG-102":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing}, withoutClientValidation)
	case "IAM-PTAG-103":
		keys := make([]string, maxTagKeys+1)
		for i := range keys {
			keys[i] = fmt.Sprintf("conf-%02d", i)
		}
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing, TagKeys: keys})
	case "IAM-PTAG-104":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{
			PolicyArn: missing, TagKeys: []string{strings.Repeat("k", maxTagKeyLen+1)},
		})
	case "IAM-PTAG-105":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing, TagKeys: []string{"conf-a"}})
	case "IAM-PTAG-106":
		foreign := policyARN(otherAccount(rt.Cfg.AccountID), rt.Name("ptag-foreign"))
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: aws.String(foreign), TagKeys: []string{"conf-a"}})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

// untagOwned creates a run-owned policy with tags, runs the setup untag pre
// when it is set, then measures UntagPolicy on keys and records the tags left.
func untagOwned(ctx context.Context, rt *harness.Runtime, suffix string, pre, keys []string,
	tags ...types.Tag,
) (o cases.Outcome, err error) {
	api := rt.Clients.IAM
	arn, err := createPolicy(ctx, rt, rt.Name(suffix))
	if err != nil {
		return cases.Outcome{}, wrapErr("untagOwned", err)
	}
	defer func() {
		err = errors.Join(err, deletePolicy(ctx, rt, arn))
	}()
	if err := setup(ctx, func(ctx context.Context) error {
		_, err := api.TagPolicy(ctx, &awsiam.TagPolicyInput{PolicyArn: aws.String(arn), Tags: tags})
		return wrapErr("TagPolicy", err)
	}); err != nil {
		return cases.Outcome{}, wrapErr("untagOwned: TagPolicy", err)
	}
	if pre != nil {
		if err := setup(ctx, func(ctx context.Context) error {
			_, err := api.UntagPolicy(ctx, &awsiam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: pre})
			return wrapErr("UntagPolicy", err)
		}); err != nil {
			return cases.Outcome{}, wrapErr("untagOwned: setup UntagPolicy", err)
		}
	}
	o, err = untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: keys})
	if err != nil || o.Code != "" {
		return o, err
	}
	left, err := listTags(ctx, rt, arn)
	if err != nil {
		return o, wrapErr("untagOwned", err)
	}
	o.Fields["tags"] = left
	return o, nil
}

func untag(ctx context.Context, rt *harness.Runtime, in *awsiam.UntagPolicyInput,
	opts ...func(*awsiam.Options),
) (cases.Outcome, error) {
	o, err := rt.Call(ctx, func(ctx context.Context) error {
		_, err := rt.Clients.IAM.UntagPolicy(ctx, in, opts...)
		return wrapErr("untag", err)
	})
	if err != nil {
		return o, wrapErr("untag", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "UntagPolicy")
	return o, nil
}

// withoutClientValidation drops the SDK's required-member check so the
// request reaches the service, which must answer the documented error itself.
func withoutClientValidation(o *awsiam.Options) {
	o.APIOptions = append(o.APIOptions, func(s *middleware.Stack) error {
		_, err := s.Initialize.Remove("OperationInputValidation")
		return wrapErr("withoutClientValidation", err)
	})
}

// createPolicy creates the run-owned policy with no tags. The name carries the
// run ID, so EntityAlreadyExists means an earlier attempt of this run left it
// behind; it is ours, and it is deleted and created again so its old tags
// cannot leak into the case.
func createPolicy(ctx context.Context, rt *harness.Runtime, name string) (string, error) {
	arn, err := createPolicyOnce(ctx, rt, name)
	var ae smithy.APIError
	if !errors.As(err, &ae) || ae.ErrorCode() != "EntityAlreadyExists" {
		return arn, wrapErr("createPolicy "+name, err)
	}
	stale, err := findPolicy(ctx, rt, name)
	if err != nil {
		return "", wrapErr("createPolicy", err)
	}
	if err := deletePolicy(ctx, rt, stale); err != nil {
		return "", wrapErr("createPolicy", err)
	}
	arn, err = createPolicyOnce(ctx, rt, name)
	return arn, wrapErr("createPolicy "+name, err)
}

func createPolicyOnce(ctx context.Context, rt *harness.Runtime, name string) (string, error) {
	var arn string
	err := setup(ctx, func(ctx context.Context) error {
		out, err := rt.Clients.IAM.CreatePolicy(ctx, &awsiam.CreatePolicyInput{
			PolicyName: aws.String(name), PolicyDocument: aws.String(policyDocument),
		})
		if err != nil {
			return wrapErr("CreatePolicy", err)
		}
		if out.Policy == nil || aws.ToString(out.Policy.Arn) == "" {
			return fmt.Errorf("CreatePolicy returned no policy ARN")
		}
		arn = aws.ToString(out.Policy.Arn)
		return nil
	})
	return arn, err
}

func findPolicy(ctx context.Context, rt *harness.Runtime, name string) (string, error) {
	pages := awsiam.NewListPoliciesPaginator(rt.Clients.IAM, &awsiam.ListPoliciesInput{Scope: types.PolicyScopeTypeLocal})
	for pages.HasMorePages() {
		var out *awsiam.ListPoliciesOutput
		if err := setup(ctx, func(ctx context.Context) error {
			var err error
			out, err = pages.NextPage(ctx)
			return wrapErr("ListPolicies", err)
		}); err != nil {
			return "", wrapErr("findPolicy "+name, err)
		}
		for _, p := range out.Policies {
			if aws.ToString(p.PolicyName) == name && aws.ToString(p.Arn) != "" {
				return aws.ToString(p.Arn), nil
			}
		}
	}
	return "", fmt.Errorf("findPolicy: %s exists but is not listed", name)
}

// deletePolicy runs on a context detached from cancellation so a stopped run
// still removes what it created.
func deletePolicy(ctx context.Context, rt *harness.Runtime, arn string) error {
	err := setup(context.WithoutCancel(ctx), func(ctx context.Context) error {
		_, err := rt.Clients.IAM.DeletePolicy(ctx, &awsiam.DeletePolicyInput{PolicyArn: aws.String(arn)})
		return wrapErr("DeletePolicy", err)
	})
	var ae smithy.APIError
	if errors.As(err, &ae) && ae.ErrorCode() == "NoSuchEntity" {
		return nil
	}
	return wrapErr("deletePolicy "+arn, err)
}

// listTags returns the policy's tags as sorted "key=value" pairs joined by commas.
func listTags(ctx context.Context, rt *harness.Runtime, arn string) (string, error) {
	var out *awsiam.ListPolicyTagsOutput
	if err := setup(ctx, func(ctx context.Context) error {
		var err error
		out, err = rt.Clients.IAM.ListPolicyTags(ctx, &awsiam.ListPolicyTagsInput{PolicyArn: aws.String(arn)})
		return wrapErr("ListPolicyTags", err)
	}); err != nil {
		return "", wrapErr("listTags", err)
	}
	if out.IsTruncated {
		return "", fmt.Errorf("listTags: %s returned a truncated page", arn)
	}
	pairs := make([]string, len(out.Tags))
	for i, t := range out.Tags {
		pairs[i] = aws.ToString(t.Key) + "=" + aws.ToString(t.Value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ","), nil
}

func setup(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, harness.OpTimeout)
	defer cancel()
	return fn(ctx)
}

func policyARN(account, name string) string {
	return fmt.Sprintf("arn:aws:iam::%s:policy/%s", account, name)
}

// otherAccount returns an account ID that is not the caller's.
func otherAccount(account string) string {
	if account == "111111111111" {
		return "222222222222"
	}
	return "111111111111"
}

func tag(k, v string) types.Tag {
	return types.Tag{Key: aws.String(k), Value: aws.String(v)}
}
