package touch

import "testing"

func TestSelectGroup(t *testing.T) {
	got := Select([]string{"s3/buckets.go", "README.md"})
	if got.Full {
		t.Fatal("full")
	}
	if len(got.Groups) != 1 || got.Groups[0].Service != "s3" || got.Groups[0].Name != "buckets" {
		t.Fatalf("%+v", got.Groups)
	}
}

func TestHarnessChangeIsFull(t *testing.T) {
	got := Select([]string{"internal/harness/allow.go"})
	if !got.Full {
		t.Fatal("want full")
	}
	if len(got.Groups) != 0 {
		t.Fatalf("groups %+v", got.Groups)
	}
}

func TestDocsDoNotSelectSuite(t *testing.T) {
	got := Select([]string{"docs/plan.md", "README.md"})
	if got.Full || len(got.Groups) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestIAMGroups(t *testing.T) {
	for _, tc := range []struct {
		file  string
		group Group
	}{
		{"iam/mfa.go", Group{Service: "iam", Name: "mfa"}},
		{"iam/users.go", Group{Service: "iam", Name: "users"}},
	} {
		got := Select([]string{tc.file})
		if got.Full || len(got.Groups) != 1 || got.Groups[0] != tc.group {
			t.Fatalf("%s: got %+v, want %+v", tc.file, got, tc.group)
		}
	}
	if got := Select([]string{"iam/group_test.go"}); !got.Full {
		t.Fatalf("iam/group_test.go: want full, got %+v", got)
	}
}

func TestSharedServiceFileIsFull(t *testing.T) {
	for _, f := range []string{"ec2/client.go", "vpc/wrap.go", "ebs/client.go", "s3/wrap.go"} {
		got := Select([]string{f})
		if !got.Full {
			t.Fatalf("%s: want full, got %+v", f, got)
		}
	}
}
