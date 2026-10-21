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

// parseKnownIssues reads "CASE-ID ISSUE-URL" lines. Blank lines and lines starting with # are
// ignored. A case ID that no group defines, a repeated ID, or a URL that is not a GitHub issue is an
// error, so a stale or mistyped entry cannot hide a failure.
func parseKnownIssues(data string, groups []bind.Group) (map[string]string, error) {
	ids := map[string]bool{}
	for _, g := range groups {
		for _, c := range g.Cases {
			ids[c.ID] = true
		}
	}
	known := map[string]string{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("known issues line %d: want \"CASE-ID ISSUE-URL\", got %q", i+1, line)
		}
		id, url := fields[0], fields[1]
		if !ids[id] {
			return nil, fmt.Errorf("known issues line %d: unknown case ID %s", i+1, id)
		}
		if !issueURL.MatchString(url) {
			return nil, fmt.Errorf("known issues line %d: %q is not a GitHub issue URL", i+1, url)
		}
		if _, dup := known[id]; dup {
			return nil, fmt.Errorf("known issues line %d: case ID %s is listed twice", i+1, id)
		}
		known[id] = url
	}
	return known, nil
}
