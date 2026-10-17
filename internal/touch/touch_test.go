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
