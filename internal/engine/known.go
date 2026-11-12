package engine

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/popododo0720/vws-conformance/internal/bind"
)

// knownIssuesFile maps a failing case ID to the issue that tracks the gap.
//
//go:embed known_issues.txt
var knownIssuesFile string

var issueURL = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/issues/[1-9][0-9]*$`)

// parseKnownIssues reads "CASE-ID ISSUE-URL [TARGET]" lines. Unscoped entries apply to all targets.
// Every entry is validated, including excluded targets, so stale or mistyped entries fail closed.
func parseKnownIssues(data string, groups []bind.Group, target string) (map[string]string, error) {
	ids := map[string]bool{}
	for _, g := range groups {
		for _, c := range g.Cases {
			ids[c.ID] = true
		}
	}
	known := map[string]string{}
	seen := map[string]bool{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 && len(fields) != 3 {
			return nil, fmt.Errorf("known issues line %d: want \"CASE-ID ISSUE-URL\" with optional TARGET, got %q", i+1, line)
		}
		id, url := fields[0], fields[1]
		if !ids[id] {
			return nil, fmt.Errorf("known issues line %d: unknown case ID %s", i+1, id)
		}
		if !issueURL.MatchString(url) {
			return nil, fmt.Errorf("known issues line %d: %q is not a GitHub issue URL", i+1, url)
		}
		if seen[id] {
			return nil, fmt.Errorf("known issues line %d: case ID %s is listed twice", i+1, id)
		}
		seen[id] = true
		scope := ""
		if len(fields) == 3 {
			scope = fields[2]
			switch scope {
			case "vws-test", "local-fake", "local-pve", "pve-test":
			default:
				return nil, fmt.Errorf("known issues line %d: unknown target %q", i+1, scope)
			}
		}
		if scope == "" || scope == target {
			known[id] = url
		}
	}
	return known, nil
}
