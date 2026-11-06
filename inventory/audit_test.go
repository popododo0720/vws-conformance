package inventory

import (
	"slices"
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

const issue = "https://github.com/popododo0720/vws-conformance/issues/15"

func waiver(api API, name string, line int) Waiver { return Waiver{Op{api, name}, issue, line} }

func TestAudit(t *testing.T) {
	inv := sample()
	synced := []cases.Case{
		{ID: "VPC-1", Service: "vpc", Op: "DescribeVpcs"},
		{ID: "IAM-1", Service: "iam", Op: "GetUser", Skip: "skipped"},
		{ID: "AUTH-1", Service: "auth", Op: "ListBuckets", Impl: "gap"},
		{ID: "AUTH-2", Service: "auth", Op: "GetCallerIdentity"},
	}
	runInstances := waiver(EC2, "RunInstances", 3)
	tests := []struct {
		name    string
		cases   []cases.Case
		waivers []Waiver
		want    []string
	}{
		{"in sync, skipped and gap cases count", synced, []Waiver{runInstances}, nil},
		{
			"uncased without a waiver", synced, nil,
			[]string{"ec2:RunInstances: implemented at 01234567 but has no case and no waiver"},
		},
		{
			"waiver for a cased operation",
			append(slices.Clone(synced), cases.Case{ID: "EC2-1", Service: "ec2", Op: "RunInstances"}),
			[]Waiver{runInstances},
			[]string{"ec2:RunInstances: waiver on line 3 is stale, case EC2-1 covers it"},
		},
		{
			"waiver for an operation console-api does not dispatch", synced,
			[]Waiver{runInstances, waiver(EC2, "CreateImage", 4)},
			[]string{"ec2:CreateImage: waiver on line 4 excuses an operation console-api does not dispatch at 01234567"},
		},
		{
			"case for an operation console-api does not dispatch",
			append(slices.Clone(synced), cases.Case{ID: "EBS-9", Service: "ebs", Op: "DescribeFoo", Impl: "gap"}),
			[]Waiver{runInstances},
			[]string{"EBS-9: case targets ec2:DescribeFoo, which console-api does not dispatch at 01234567"},
		},
		{
			"auth case no API dispatches",
			append(slices.Clone(synced), cases.Case{ID: "AUTH-3", Service: "auth", Op: "Nope"}),
			[]Waiver{runInstances},
			[]string{`AUTH-3: auth case targets "Nope", which no console-api API dispatches at 01234567`},
		},
		{
			"unknown service",
			append(slices.Clone(synced), cases.Case{ID: "X-1", Service: "kms", Op: "Decrypt"}),
			[]Waiver{runInstances},
			[]string{`X-1: case service "kms" is not one of ec2, vpc, ebs, s3, iam, sts, auth`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Audit(inv, tt.waivers, tt.cases); !slices.Equal(got, tt.want) {
				t.Errorf("findings\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestAuditAmbiguousAuthOp(t *testing.T) {
	inv := Inventory{Commit: fixtureCommit, Ops: map[Op]bool{{IAM, "Dup"}: true, {STS, "Dup"}: true}}
	got := Audit(inv, nil, []cases.Case{{ID: "AUTH-1", Service: "auth", Op: "Dup"}})
	want := `AUTH-1: auth case op "Dup" is dispatched by [iam:Dup sts:Dup], so its API is ambiguous`
	if !slices.Contains(got, want) {
		t.Errorf("findings %q, want %q", got, want)
	}
}

func TestParseWaivers(t *testing.T) {
	good := "# comment\n\nec2:RunInstances\t" + issue + "\n  s3:GetObject\t" + issue + "  \n"
	ws, err := ParseWaivers(good)
	if err != nil || len(ws) != 2 || ws[0].Line != 3 || ws[1].Op.String() != "s3:GetObject" {
		t.Fatalf("ParseWaivers = %v, %v", ws, err)
	}
	bad := map[string]string{
		"one field":          "ec2:RunInstances\n",
		"three fields":       "ec2:RunInstances\t" + issue + "\textra\n",
		"spaces not a tab":   "ec2:RunInstances " + issue + "\n",
		"unknown api":        "kms:Decrypt\t" + issue + "\n",
		"old dotted form":    "ec2.RunInstances\t" + issue + "\n",
		"repeated op":        "ec2:RunInstances\t" + issue + "\nec2:RunInstances\t" + issue + "\n",
		"other host":         "ec2:RunInstances\thttps://example.com/popododo0720/vws-conformance/issues/15\n",
		"other owner":        "ec2:RunInstances\thttps://github.com/other/vws-conformance/issues/15\n",
		"not a vws repo":     "ec2:RunInstances\thttps://github.com/popododo0720/other/issues/15\n",
		"pull request":       "ec2:RunInstances\thttps://github.com/popododo0720/vws-conformance/pull/15\n",
		"query string":       "ec2:RunInstances\t" + issue + "?x=1\n",
		"zero issue number":  "ec2:RunInstances\thttps://github.com/popododo0720/vws-conformance/issues/0\n",
		"plain http":         "ec2:RunInstances\thttp://github.com/popododo0720/vws-conformance/issues/15\n",
		"trailing path part": "ec2:RunInstances\t" + issue + "/comments\n",
	}
	for name, data := range bad {
		if _, err := ParseWaivers(data); err == nil || !strings.Contains(err.Error(), "waivers line") {
			t.Errorf("%s: error %v", name, err)
		}
	}
}
