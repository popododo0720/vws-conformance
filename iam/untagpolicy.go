package iam

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"

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

func untagCases() []cases.Case {
	return []cases.Case{
		untagCase("IAM-PTAG-001", cases.KindTags, cases.DocIAM("UntagPolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "tags": `^conf-c=3$`},
		}),
		untagCase("IAM-PTAG-002", cases.KindIdempotency, cases.DocIAM("UntagPolicy"), cases.Expect{
			HTTP: 200, Patterns: map[string]string{"shape": expect.ShapeOKPattern, "tags": `^conf-a=1$`},
		}),
		untagCase("IAM-PTAG-103", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-104", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-105", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-106", cases.KindError, cases.DocIAMErrors, cases.Expect{HTTP: 400, Code: "ValidationError"}),
		untagCase("IAM-PTAG-107", cases.KindError, cases.DocIAM("UntagPolicy"),
			cases.Expect{HTTP: 404, Code: "NoSuchEntity"}),
		untagCase("IAM-PTAG-108", cases.KindAuth, cases.DocIAMCrossAccount,
			cases.Expect{HTTP: 403, Code: "AccessDenied"}),
		{
			ID: "IAM-PTAG-109", Service: "iam", Group: "policytags", Op: "UntagPolicy", Kind: cases.KindAuth,
			Suite: cases.SuiteSDK, DocURL: cases.DocIAMErrors,
			Expect: cases.Expect{HTTP: 403, Code: "AccessDenied"}, Skip: cases.SkipUntagPolicyAuth, Impl: "yes",
		},
	}
}

func untagCase(id, kind, doc string, exp cases.Expect) cases.Case {
	return cases.Case{
		ID: id, Service: "iam", Group: "policytags", Op: "UntagPolicy", Kind: kind,
		Suite: cases.SuiteSDK, DocURL: doc, Expect: exp, Impl: "yes",
	}
}

func runUntagPolicy(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
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
	case "IAM-PTAG-103":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{TagKeys: []string{"conf-a"}})
	case "IAM-PTAG-104":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing})
	case "IAM-PTAG-105":
		keys := make([]string, maxTagKeys+1)
		for i := range keys {
			keys[i] = fmt.Sprintf("conf-%02d", i)
		}
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing, TagKeys: keys})
	case "IAM-PTAG-106":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{
			PolicyArn: missing, TagKeys: []string{strings.Repeat("k", maxTagKeyLen+1)},
		})
	case "IAM-PTAG-107":
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: missing, TagKeys: []string{"conf-a"}})
	case "IAM-PTAG-108":
		foreign := policyARN(otherAccount(rt.Cfg.AccountID), rt.Name("ptag-foreign"))
		return untag(ctx, rt, &awsiam.UntagPolicyInput{PolicyArn: aws.String(foreign), TagKeys: []string{"conf-a"}})
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

// untagOwned creates a run-owned policy with tags, runs the setup untag pre
// when it is set, then measures UntagPolicy on keys and records the tags left.
// The gate deletes the policy when the case ends.
func untagOwned(ctx context.Context, rt *harness.Runtime, suffix string, pre, keys []string,
	tags ...types.Tag,
) (cases.Outcome, error) {
	arn, err := createPolicy(ctx, rt, rt.Name(suffix))
	if err != nil {
		return cases.Outcome{}, wrapErr("untagOwned", err)
	}
	if err := setup(ctx, func(ctx context.Context) error {
		_, err := rt.IAMTagPolicy(ctx, &awsiam.TagPolicyInput{PolicyArn: aws.String(arn), Tags: tags})
		return wrapErr("IAMTagPolicy", err)
	}); err != nil {
		return cases.Outcome{}, wrapErr("untagOwned: TagPolicy", err)
	}
	if pre != nil {
		if err := setup(ctx, func(ctx context.Context) error {
			_, err := rt.IAMUntagPolicy(ctx, &awsiam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: pre})
			return wrapErr("IAMUntagPolicy", err)
		}); err != nil {
			return cases.Outcome{}, wrapErr("untagOwned: setup UntagPolicy", err)
		}
	}
	o, err := measure(ctx, rt, func(ctx context.Context) error {
		_, err := rt.IAMUntagPolicy(ctx, &awsiam.UntagPolicyInput{PolicyArn: aws.String(arn), TagKeys: keys})
		return wrapErr("IAMUntagPolicy", err)
	})
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

// untag measures an UntagPolicy the service must reject.
func untag(ctx context.Context, rt *harness.Runtime, in *awsiam.UntagPolicyInput) (cases.Outcome, error) {
	return measure(ctx, rt, func(ctx context.Context) error {
		_, err := rt.IAMProbeUntagPolicy(ctx, in)
		return wrapErr("IAMProbeUntagPolicy", err)
	})
}

func measure(ctx context.Context, rt *harness.Runtime, call func(context.Context) error) (cases.Outcome, error) {
	o, err := rt.Call(ctx, call)
	if err != nil {
		return o, wrapErr("UntagPolicy", err)
	}
	o.Fields["shape"] = expect.QueryNoResultField(o, "UntagPolicy")
	return o, nil
}

// createPolicy creates the run-owned policy. The gate deletes it when the case ends.
func createPolicy(ctx context.Context, rt *harness.Runtime, name string) (string, error) {
	var arn string
	err := setup(ctx, func(ctx context.Context) error {
		out, err := rt.IAMCreatePolicy(ctx, &awsiam.CreatePolicyInput{
			PolicyName: aws.String(name), PolicyDocument: aws.String(policyDocument),
		})
		if err != nil {
			return wrapErr("IAMCreatePolicy", err)
		}
		arn = aws.ToString(out.Policy.Arn)
		return nil
	})
	return arn, err
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
