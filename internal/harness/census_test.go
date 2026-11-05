package harness

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// actFile is the only module-relative path allowed to select a mutating SDK method, and only as
// a direct call that passes oneAttempt.
const actFile = "internal/harness/act.go"

func TestMutatingSDKMethodsStayInAct(t *testing.T) {
	if bad := violations(mutatingSelections(t, nil)); len(bad) > 0 {
		t.Fatalf("mutating SDK method outside %s, or sent with SDK retries:\n%s", actFile, strings.Join(bad, "\n"))
	}
}

func TestCensusReportsMutationOutsideAct(t *testing.T) {
	root := moduleRoot(t)
	buckets := filepath.Join(root, "s3", "buckets.go")
	body, err := os.ReadFile(buckets)
	if err != nil {
		t.Fatal(err)
	}
	act := filepath.Join(root, filepath.FromSlash(actFile))
	actBody, err := os.ReadFile(act)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		file   string
		body   string
		report string
	}{
		{"service package", buckets, string(body) + `
func dropBucket(ctx context.Context, api *s3.Client) {
	_, _ = api.DeleteBucket(ctx, nil)
}
`, "s3/buckets.go: DeleteBucket"},
		{"new file beside act.go", filepath.Join(root, "internal", "harness", "act_extra.go"), `package harness

import "github.com/aws/aws-sdk-go-v2/service/ec2"

func extraTerminate(api *ec2.Client) { _ = api.TerminateInstances }
`, "internal/harness/act_extra.go: TerminateInstances"},
		{"act.go call with SDK retries", act, string(actBody) + `
func actTerminate(ctx context.Context, api *ec2.Client) { _, _ = api.TerminateInstances(ctx, nil) }
`, actFile + ": TerminateInstances without oneAttempt"},
		{"act.go method value", act, string(actBody) + `
func actTerminate(api *ec2.Client) { _ = api.TerminateInstances }
`, actFile + ": TerminateInstances without oneAttempt"},
		{"act.go call with oneAttempt", act, string(actBody) + `
func actTerminate(ctx context.Context, api *ec2.Client) { _, _ = api.TerminateInstances(ctx, nil, oneAttempt) }
`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all := mutatingSelections(t, map[string][]byte{tt.file: []byte(tt.body)})
			bad := violations(all)
			if tt.report == "" {
				if !slices.Contains(all, hit{actFile, "TerminateInstances", true}) || len(bad) != 0 {
					t.Fatalf("act.go hit was not seen, or something was reported:\n%s", strings.Join(bad, "\n"))
				}
				return
			}
			if !slices.Contains(bad, tt.report) {
				t.Fatalf("%q was not reported:\n%s", tt.report, strings.Join(bad, "\n"))
			}
		})
	}
}

// hit is one selection of a mutating SDK method. oneAttempt reports that the selection is
// called directly with oneAttempt as its last argument.
type hit struct {
	file       string
	method     string
	oneAttempt bool
}

func mutatingSelections(t *testing.T, overlay map[string][]byte) []hit {
	t.Helper()
	root := moduleRoot(t)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir:     root,
		Env:     os.Environ(),
		Overlay: overlay,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}
	var hits []hit
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			t.Fatalf("%s: %v", pkg.PkgPath, pkg.Errors)
		}
		for _, file := range pkg.Syntax {
			name := pkg.Fset.Position(file.Pos()).Filename
			rel, err := filepath.Rel(root, name)
			if err != nil {
				t.Fatal(err)
			}
			rel = filepath.ToSlash(rel)
			if strings.HasSuffix(rel, "_test.go") {
				continue
			}
			once := map[*ast.SelectorExpr]bool{}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				last, isIdent := call.Args[len(call.Args)-1].(*ast.Ident)
				if ok && isIdent && last.Name == "oneAttempt" {
					once[sel] = true
				}
				return true
			})
			ast.Inspect(file, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || pkg.TypesInfo == nil {
					return true
				}
				use := pkg.TypesInfo.Selections[sel]
				if use == nil || use.Obj() == nil {
					return true
				}
				if !mutatingClient(use.Recv()) || readAPI(use.Obj().Name()) {
					return true
				}
				hits = append(hits, hit{rel, use.Obj().Name(), once[sel]})
				return true
			})
		}
	}
	return hits
}

func violations(hits []hit) []string {
	var out []string
	for _, h := range hits {
		switch {
		case h.file != actFile:
			out = append(out, fmt.Sprintf("%s: %s", h.file, h.method))
		case !h.oneAttempt:
			out = append(out, fmt.Sprintf("%s: %s without oneAttempt", h.file, h.method))
		}
	}
	return out
}

func mutatingClient(recv interface{ String() string }) bool {
	s := recv.String()
	for _, path := range []string{
		"github.com/aws/aws-sdk-go-v2/service/ec2.Client",
		"github.com/aws/aws-sdk-go-v2/service/s3.Client",
		"github.com/aws/aws-sdk-go-v2/service/iam.Client",
		"github.com/aws/aws-sdk-go-v2/service/sts.Client",
	} {
		if s == "*"+path {
			return true
		}
	}
	return false
}

func readAPI(name string) bool {
	switch {
	case strings.HasPrefix(name, "Describe"),
		strings.HasPrefix(name, "Get"),
		strings.HasPrefix(name, "List"),
		strings.HasPrefix(name, "Head"),
		strings.HasPrefix(name, "Search"),
		name == "Options":
		return true
	default:
		return false
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
