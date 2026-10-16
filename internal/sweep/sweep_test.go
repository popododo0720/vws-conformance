package sweep

import "testing"

func TestSelectOnlySuitePrefix(t *testing.T) {
	in := []Resource{
		{Name: "conf-20261015t120000z-abc123-b1"},
		{Name: "prod-logs"},
		{RunTag: "conf-20261015T120000Z-abc123", ID: "i-abc"},
		{RunTag: "team", ID: "i-def"},
		{Path: "/conformance/user"},
		{Path: "/admin/"},
		{Description: "conf-20261015t120000z-abc123-snap"},
		{Description: "daily"},
		{ID: "vol-abc", LedgerRunID: "conf-20261015T120000Z-abc123"},
		{ID: "vol-def", LedgerRunID: "manual"},
		{ID: "vol-ghi"},
	}
	got := Select(in)
	if len(got) != 5 {
		t.Fatalf("selected %d: %+v", len(got), got)
	}
	for _, r := range got {
		if r.Name == "prod-logs" || r.ID == "vol-def" || r.ID == "vol-ghi" || r.ID == "i-def" {
			t.Fatalf("selected foreign resource %+v", r)
		}
	}
}
