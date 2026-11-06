package inventory

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"
)

//go:embed waivers.tsv
var waiversFile string

// issueURL is a GitHub issue of a vws repository.
var issueURL = regexp.MustCompile(`^https://github\.com/popododo0720/vws-[a-z]+(-[a-z]+)*/issues/[1-9][0-9]*$`)

// Waiver excuses an implemented operation from having a case until Issue is resolved.
type Waiver struct {
	Op    Op
	Issue string
	Line  int
}

// ParseWaivers reads "api:Name<TAB>issue URL" lines. Blank lines and lines starting with # are
// ignored. A malformed line, an unknown API, a URL that is not a vws GitHub issue and a repeated
// operation are errors, so a mistyped waiver cannot excuse anything.
func ParseWaivers(data string) ([]Waiver, error) {
	var out []Waiver
	seen := map[Op]int{}
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 2 {
			return nil, fmt.Errorf("waivers line %d: want \"api:Name<TAB>issue URL\", got %q", i+1, line)
		}
		op, err := ParseOp(fields[0])
		if err != nil {
			return nil, fmt.Errorf("waivers line %d: %w", i+1, err)
		}
		if !issueURL.MatchString(fields[1]) {
			return nil, fmt.Errorf("waivers line %d: %q is not a vws GitHub issue URL", i+1, fields[1])
		}
		if first, dup := seen[op]; dup {
			return nil, fmt.Errorf("waivers line %d: %s is already waived on line %d", i+1, op, first)
		}
		seen[op] = i + 1
		out = append(out, Waiver{op, fields[1], i + 1})
	}
	return out, nil
}
