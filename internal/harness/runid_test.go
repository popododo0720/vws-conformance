package harness

import (
	"strings"
	"testing"
	"time"
)

func TestRunIDPrefixOnNames(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	id, err := NewRunID(now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "conf-20261015T120000Z-") {
		t.Fatalf("id %s", id)
	}
	name := ResourceName(id, "b1")
	if name != strings.ToLower(id)+"-b1" {
		t.Fatalf("name %s", name)
	}
	if strings.ContainsAny(name, "TZ") {
		t.Fatalf("bucket name has uppercase %s", name)
	}
	if !strings.HasPrefix(name, "conf-") {
		t.Fatal(name)
	}
}

func TestDefaultParallelIsOne(t *testing.T) {
	n, err := ParseParallel("")
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	n, err = ParseParallel("2")
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
