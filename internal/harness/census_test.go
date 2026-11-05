package harness

import (
	"fmt"
	"go/ast"
	"go/types"
	"io/fs"
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
	if bad := runCensus(t, nil).violations(); len(bad) > 0 {
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
		{"value receiver", filepath.Join(root, "s3", "value_client.go"), `package s3

import "github.com/aws/aws-sdk-go-v2/service/ec2"

func valueDelete(api ec2.Client) { _ = api.DeleteVolume }
`, "s3/value_client.go: DeleteVolume"},
		{"embedded client", filepath.Join(root, "s3", "embedded_client.go"), `package s3

import "github.com/aws/aws-sdk-go-v2/service/ec2"

type wrapped struct{ *ec2.Client }

func embeddedDelete(w wrapped) { _ = w.DeleteVolume }
`, "s3/embedded_client.go: DeleteVolume"},
		{"interface holding a client", filepath.Join(root, "s3", "interface_client.go"), `package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type deleter interface {
	DeleteVolume(context.Context, *ec2.DeleteVolumeInput, ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error)
}

func interfaceDelete(ctx context.Context, api *ec2.Client) {
	var d deleter = api
	_, _ = d.DeleteVolume(ctx, nil)
}
`, "s3/interface_client.go: DeleteVolume"},
		{"file behind a build tag", filepath.Join(root, "s3", "tagged.go"), `//go:build census

package s3

import "github.com/aws/aws-sdk-go-v2/service/ec2"

func taggedDelete(api *ec2.Client) { _ = api.DeleteVolume }
`, "s3/tagged.go: not compiled under the default build"},
		{"act.go call with SDK retries", act, string(actBody) + `
func actTerminate(ctx context.Context, api *ec2.Client) { _, _ = api.TerminateInstances(ctx, nil) }
`, actFile + ": TerminateInstances without oneAttempt"},
		{"act.go method value", act, string(actBody) + `
func actTerminate(api *ec2.Client) { _ = api.TerminateInstances }
`, actFile + ": TerminateInstances without oneAttempt"},
		{"act.go call with a local oneAttempt", act, string(actBody) + `
func actTerminate(ctx context.Context, api *ec2.Client) {
	oneAttempt := func(o *ec2.Options) { o.RetryMaxAttempts = 3 }
	_, _ = api.TerminateInstances(ctx, nil, oneAttempt)
}
`, actFile + ": TerminateInstances without oneAttempt"},
		{"act.go call with oneAttempt", act, string(actBody) + `
func actTerminate(ctx context.Context, api *ec2.Client) { _, _ = api.TerminateInstances(ctx, nil, oneAttempt) }
`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := runCensus(t, map[string][]byte{tt.file: []byte(tt.body)})
			bad := c.violations()
			if tt.report == "" {
				if !slices.Contains(c.hits, hit{actFile, "TerminateInstances", true}) || len(bad) != 0 {
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

// clientPackages hold the SDK clients whose mutating methods only act.go may call.
var clientPackages = []string{
	"github.com/aws/aws-sdk-go-v2/service/ec2",
	"github.com/aws/aws-sdk-go-v2/service/s3",
	"github.com/aws/aws-sdk-go-v2/service/iam",
	"github.com/aws/aws-sdk-go-v2/service/sts",
}

// hit is one selection of a mutating SDK method. oneAttempt reports that the selection is
// called directly with oneAttempt as its last argument.
type hit struct {
	file       string
	method     string
	oneAttempt bool
}

type census struct {
	hits       []hit
	uncompiled []string // module-relative non-test files the default build does not compile
}

func (c census) violations() []string {
	var out []string
	for _, h := range c.hits {
		switch {
		case h.file != actFile:
			out = append(out, fmt.Sprintf("%s: %s", h.file, h.method))
		case !h.oneAttempt:
			out = append(out, fmt.Sprintf("%s: %s without oneAttempt", h.file, h.method))
		}
	}
	for _, f := range c.uncompiled {
		out = append(out, f+": not compiled under the default build")
	}
	return out
}

func runCensus(t *testing.T, overlay map[string][]byte) census {
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
	mutating := mutatingNames(t, pkgs)
	var c census
	compiled := map[string]bool{}
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			t.Fatalf("%s: %v", pkg.PkgPath, pkg.Errors)
		}
		for _, f := range pkg.GoFiles {
			compiled[f] = true
		}
		for _, file := range pkg.Syntax {
			rel := relPath(t, root, pkg.Fset.Position(file.Pos()).Filename)
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
				if ok && isIdent && isPackageOneAttempt(pkg, last) {
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
				if use == nil {
					return true
				}
				fn, ok := use.Obj().(*types.Func)
				if !ok || readAPI(fn.Name()) {
					return true
				}
				sig, ok := fn.Type().(*types.Signature)
				if !ok || sig.Recv() == nil {
					return true
				}
				recv := sig.Recv().Type()
				if clientType(recv) || types.IsInterface(recv) && mutating[fn.Name()] {
					c.hits = append(c.hits, hit{rel, fn.Name(), once[sel]})
				}
				return true
			})
		}
	}
	for _, f := range sourceFiles(t, root, overlay) {
		if !compiled[f] {
			c.uncompiled = append(c.uncompiled, relPath(t, root, f))
		}
	}
	return c
}

// clientType reports whether t, pointer stripped, is the Client of a client package. It is read
// from the method's own receiver, so value, pointer and embedded selections all match.
func clientType(t types.Type) bool {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Name() != "Client" || named.Obj().Pkg() == nil {
		return false
	}
	return slices.Contains(clientPackages, named.Obj().Pkg().Path())
}

// mutatingNames is every non-read method of the four clients. A call through an interface the
// client was assigned to selects the interface's method, so it is matched by name.
func mutatingNames(t *testing.T, pkgs []*packages.Package) map[string]bool {
	t.Helper()
	clients := map[string]*types.Package{}
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		for _, imp := range pkg.Types.Imports() {
			if slices.Contains(clientPackages, imp.Path()) {
				clients[imp.Path()] = imp
			}
		}
	}
	names := map[string]bool{}
	for _, path := range clientPackages {
		pkg := clients[path]
		if pkg == nil {
			t.Fatalf("no module package imports %s, so its mutating methods are unknown", path)
		}
		obj, ok := pkg.Scope().Lookup("Client").(*types.TypeName)
		if !ok {
			t.Fatalf("%s has no Client type", path)
		}
		methods := types.NewMethodSet(types.NewPointer(obj.Type()))
		for i := range methods.Len() {
			if name := methods.At(i).Obj().Name(); !readAPI(name) {
				names[name] = true
			}
		}
	}
	return names
}

// sourceFiles lists every non-test .go file the go command would consider part of the module,
// overlay files included.
func sourceFiles(t *testing.T, root string, overlay map[string][]byte) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path := range overlay {
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && !slices.Contains(files, path) {
			files = append(files, path)
		}
	}
	return files
}

func relPath(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
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

// isPackageOneAttempt reports that id resolves to the package-level oneAttempt of the harness,
// not a local value that happens to share its name.
func isPackageOneAttempt(pkg *packages.Package, id *ast.Ident) bool {
	obj, ok := pkg.TypesInfo.Uses[id].(*types.Func)
	return ok && obj.Name() == "oneAttempt" && obj.Pkg() == pkg.Types && obj.Parent() == pkg.Types.Scope()
}
