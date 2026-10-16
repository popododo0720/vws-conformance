package vpc

import (
	"errors"
	"testing"
)

func TestIgnoreGone(t *testing.T) {
	if err := ignoreGone(errors.New("api error InvalidGroup.NotFound: not found")); err != nil {
		t.Fatal(err)
	}
	if ignoreGone(nil) != nil {
		t.Fatal("nil")
	}
	if err := ignoreGone(errors.New("api error DependencyViolation: vpc has dependents")); err == nil {
		t.Fatal("dependency was ignored")
	}
}
