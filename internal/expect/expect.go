package expect

import (
	"fmt"
	"regexp"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

func Judge(exp cases.Expect, got cases.Outcome) (status, reason string) {
	if exp.Head {
		if got.HTTP != exp.HTTP {
			return "fail", fmt.Sprintf("http got %d want %d", got.HTTP, exp.HTTP)
		}
		return "pass", ""
	}
	if exp.HTTP != 0 && got.HTTP != exp.HTTP {
		return "fail", fmt.Sprintf("http got %d want %d, code %q", got.HTTP, exp.HTTP, got.Code)
	}
	if got.Code != exp.Code {
		return "fail", fmt.Sprintf("code got %q want %q", got.Code, exp.Code)
	}
	if exp.Code != "" && got.Message == "" {
		return "fail", "error message is empty"
	}
	for field, pat := range exp.Patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			return "error", fmt.Sprintf("pattern %s: %v", field, err)
		}
		if !re.MatchString(got.Fields[field]) {
			return "fail", fmt.Sprintf("field %s got %q, pattern %s", field, got.Fields[field], pat)
		}
	}
	return "pass", ""
}
