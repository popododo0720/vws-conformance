package sweep

import "strings"

type Resource struct {
	Kind        string
	ID          string
	Name        string
	RunTag      string
	Path        string
	Description string
	LedgerRunID string
}

func CarriesPrefix(r Resource) bool {
	if strings.HasPrefix(r.Name, "conf-") {
		return true
	}
	if strings.HasPrefix(r.RunTag, "conf-") {
		return true
	}
	if strings.HasPrefix(r.Path, "/conformance/") {
		return true
	}
	if strings.HasPrefix(r.Description, "conf-") {
		return true
	}
	if strings.HasPrefix(r.LedgerRunID, "conf-") && r.ID != "" {
		return true
	}
	return false
}

func Select(in []Resource) []Resource {
	var out []Resource
	for _, r := range in {
		if CarriesPrefix(r) {
			out = append(out, r)
		}
	}
	return out
}
