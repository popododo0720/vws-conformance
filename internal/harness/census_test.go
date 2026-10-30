package harness

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestProductionCodeHasNoMutatingSDKMethod(t *testing.T) {
	hits := mutatingSelections(t, nil)
	if len(hits) > 0 {
		t.Fatalf("mutating SDK method in production code:\n%s", strings.Join(hits, "\n"))
	}
}

func TestCensusCatchesMethodValueAlias(t *testing.T) {
	root := moduleRoot(t)
	target := filepath.Join(root, "s3", "buckets.go")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	alias := string(body) + `
func holdCreate(api *s3.Client) {
	fn := api.CreateBucket
	_ = fn
}
`
	hits := mutatingSelections(t, map[string][]byte{target: []byte(alias)})
	for _, hit := range hits {
		if strings.Contains(hit, "buckets.go") && strings.Contains(hit, "CreateBucket") {
			return
		}
	}
	t.Fatalf("method value alias was not a mutating selection:\n%s", strings.Join(hits, "\n"))
}

func mutatingSelections(t *testing.T, overlay map[string][]byte) []string {
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
	var hits []string
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
				if selfCleaningBucketCall(rel, use.Obj().Name()) {
					return true
				}
				hits = append(hits, fmt.Sprintf("%s: %s", rel, use.Obj().Name()))
				return true
			})
		}
	}
	return hits
}

func TestCensusAllowsOnlyBucketLifecycleMutations(t *testing.T) {
	root := moduleRoot(t)
	target := filepath.Join(root, "s3", "bucket_lifecycle.go")
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	overlay := string(body) + `
func holdPut(api *s3.Client) {
	_ = api.PutObject
}
`
	hits := mutatingSelections(t, map[string][]byte{target: []byte(overlay)})
	for _, hit := range hits {
		if strings.Contains(hit, "bucket_lifecycle.go") && strings.Contains(hit, "PutObject") {
			return
		}
	}
	t.Fatalf("a bucket lifecycle mutation other than create or delete was not caught:\n%s", strings.Join(hits, "\n"))
}

// selfCleaningBucketCall is the one mutation the census permits outside the
// cleanup gate (popododo0720/vws-conformance#5). The S3 bucket lifecycle cases
// use run-scoped names and register cleanup in the per-case cleaner, so a
// failed case still deletes only its own bucket.
func selfCleaningBucketCall(rel, method string) bool {
	if rel != "s3/bucket_lifecycle.go" {
		return false
	}
	return method == "CreateBucket" || method == "DeleteBucket"
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
