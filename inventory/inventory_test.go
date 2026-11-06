package inventory

import (
	"bytes"
	"strings"
	"testing"
)

func sample() Inventory {
	return Inventory{Commit: fixtureCommit, Ops: map[Op]bool{
		{EC2, "RunInstances"}: true, {EC2, "DescribeVpcs"}: true, {IAM, "GetUser"}: true,
		{S3, "ListBuckets"}: true, {STS, "GetCallerIdentity"}: true,
	}}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	a, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	inv, err := Decode(a)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode(inv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("re-encode differs:\n%s\n%s", a, b)
	}
	text := string(a)
	if !strings.HasSuffix(text, "}\n") || strings.Index(text, "DescribeVpcs") > strings.Index(text, "RunInstances") {
		t.Errorf("not canonical:\n%s", a)
	}
}

func TestDecodeRejectsDrift(t *testing.T) {
	good, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, old, new string }{
		{"unsorted", `"DescribeVpcs",` + "\n      " + `"RunInstances"`, `"RunInstances",` + "\n      " + `"DescribeVpcs"`},
		{"repeated", `"DescribeVpcs",`, `"DescribeVpcs", "DescribeVpcs",`},
		{"unknown api", `"sts"`, `"kms"`},
		{"empty api", `"GetCallerIdentity"`, ``},
		{"short commit", fixtureCommit, "0123abcd"},
		{"uppercase commit", fixtureCommit, strings.ToUpper(fixtureCommit)},
		{"wrong schema", `"schema_version": "1"`, `"schema_version": "2"`},
		{"wrong repo", `popododo0720/vws-console-api`, `other/repo`},
		{"unknown field", `"schema_version"`, `"extra": 1, "schema_version"`},
		{"bad name", `"GetUser"`, `"Get User"`},
		{"trailing data", `"1"`, `"1"} {`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(string(good), tt.old) {
				t.Fatalf("fixture lacks %q", tt.old)
			}
			if _, err := Decode([]byte(strings.Replace(string(good), tt.old, tt.new, 1))); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if _, err := Decode(nil); err == nil {
		t.Error("empty input: want an error")
	}
}

func TestEncodeRejectsBadInventory(t *testing.T) {
	for name, inv := range map[string]Inventory{
		"bad commit":  {Commit: "x", Ops: sample().Ops},
		"unknown api": {Commit: fixtureCommit, Ops: map[Op]bool{{"kms", "Decrypt"}: true}},
		"empty api":   {Commit: fixtureCommit, Ops: map[Op]bool{{EC2, "RunInstances"}: true}},
	} {
		if _, err := Encode(inv); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestParseOp(t *testing.T) {
	for _, s := range []string{"ec2:RunInstances", "sts:GetCallerIdentity"} {
		if op, err := ParseOp(s); err != nil || op.String() != s {
			t.Errorf("ParseOp(%q) = %v, %v", s, op, err)
		}
	}
	for _, s := range []string{"", "ec2", "ec2:", "kms:Decrypt", "ec2.RunInstances", "ec2:Run Instances"} {
		if _, err := ParseOp(s); err == nil {
			t.Errorf("ParseOp(%q): want an error", s)
		}
	}
}
