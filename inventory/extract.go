package inventory

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	fileOperation = "internal/resource/operation.go"
	fileEC2       = "internal/surface/ec2query/ec2query.go"
	fileS3        = "internal/surface/s3rest/s3rest.go"
)

// s3RouteOps names the AWS operations each S3 mux pattern serves. A pattern is not an operation
// name, and GET /{bucket} serves ListObjects and ListObjectsV2 by its list-type query. Extraction
// fails when this table and the mux disagree in either direction.
var s3RouteOps = map[string][]string{
	"GET /{$}":                  {"ListBuckets"},
	"GET /{bucket}":             {"ListObjects", "ListObjectsV2"},
	"HEAD /{bucket}":            {"HeadBucket"},
	"PUT /{bucket}":             {"CreateBucket"},
	"DELETE /{bucket}":          {"DeleteBucket"},
	"PUT /{bucket}/{key...}":    {"PutObject"},
	"GET /{bucket}/{key...}":    {"GetObject"},
	"HEAD /{bucket}/{key...}":   {"HeadObject"},
	"DELETE /{bucket}/{key...}": {"DeleteObject"},
}

// source is one parsed console-api file. It exists so every shape error names file:line.
type source struct {
	path string
	fset *token.FileSet
	file *ast.File
}

func (s source) errorf(n ast.Node, format string, args ...any) error {
	return fmt.Errorf("%s:%d: %s", s.path, s.fset.Position(n.Pos()).Line, fmt.Sprintf(format, args...))
}

func parseSource(dir, rel string) (source, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(dir, filepath.FromSlash(rel)), nil, parser.SkipObjectResolution)
	if err != nil {
		return source{}, fmt.Errorf("parse %s: %w", rel, err)
	}
	return source{rel, fset, f}, nil
}

// funcDecl finds the function name, or the method name on receiver type recv ("" for a function).
func (s source) funcDecl(name, recv string) (*ast.FuncDecl, error) {
	var found *ast.FuncDecl
	for _, d := range s.file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name || receiver(fn) != recv {
			continue
		}
		if found != nil {
			return nil, s.errorf(fn, "second declaration of %s", name)
		}
		found = fn
	}
	if found == nil || found.Body == nil {
		return nil, fmt.Errorf("%s: no function %s", s.path, name)
	}
	return found, nil
}

func receiver(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	return strings.TrimPrefix(types.ExprString(fn.Recv.List[0].Type), "*")
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	return v, err == nil
}

// Extract reads the console-api checkout in dir and lists what it dispatches. It parses three files
// and executes nothing, because the egress census forbids os/exec in production code. A source
// shape it does not recognize is an error naming file:line, so a console-api refactor fails the
// tool instead of silently dropping operations.
func Extract(dir string) (Inventory, error) {
	commit, err := headCommit(dir)
	if err != nil {
		return Inventory{}, err
	}
	inv := Inventory{Commit: commit, Ops: map[Op]bool{}}
	for _, read := range []func(string) (map[API][]string, error){readEC2, readS3} {
		found, err := read(dir)
		if err != nil {
			return Inventory{}, err
		}
		for api, names := range found {
			for _, n := range names {
				inv.Ops[Op{api, n}] = true
			}
		}
	}
	return inv, nil
}

func readEC2(dir string) (map[API][]string, error) {
	opSrc, err := parseSource(dir, fileOperation)
	if err != nil {
		return nil, err
	}
	ec2Src, err := parseSource(dir, fileEC2)
	if err != nil {
		return nil, err
	}
	names, err := actionNames(opSrc)
	if err != nil {
		return nil, err
	}
	ops, err := ec2Dispatch(ec2Src, names)
	return map[API][]string{EC2: ops}, err
}

func readS3(dir string) (map[API][]string, error) {
	src, err := parseSource(dir, fileS3)
	if err != nil {
		return nil, err
	}
	ops, err := s3Dispatch(src)
	return map[API][]string{S3: ops}, err
}

// headCommit reads the commit a detached checkout points at without running git. A worktree's
// .git file is followed. A symbolic HEAD is an error: the tool wants the pinned commit itself.
func headCommit(dir string) (string, error) {
	gitDir := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitDir); err == nil && !info.IsDir() {
		b, err := os.ReadFile(gitDir)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", gitDir, err)
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return "", fmt.Errorf("%s: want a \"gitdir: PATH\" line", gitDir)
		}
		gitDir = target
		if !filepath.IsAbs(gitDir) {
			gitDir = filepath.Join(dir, gitDir)
		}
	}
	b, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", fmt.Errorf("read HEAD of %s: %w", dir, err)
	}
	head := strings.TrimSpace(string(b))
	if !commitRE.MatchString(head) {
		return "", fmt.Errorf("%s: HEAD is %q; check out the commit detached (git checkout --detach SHA)", dir, head)
	}
	return head, nil
}

// actionNames reads Action.String, a switch of `case ActionX: return "X"` clauses, into
// {"ActionX": "X"}. Clauses that return "" (the zero action and default) name no operation.
func actionNames(s source) (map[string]string, error) {
	fn, err := s.funcDecl("String", "Action")
	if err != nil {
		return nil, err
	}
	sw, ok := soleSwitch(fn.Body)
	if !ok {
		return nil, s.errorf(fn, "Action.String is not a single switch")
	}
	names := map[string]string{}
	for _, cc := range clauses(sw) {
		name, ok := returnedString(cc)
		if !ok {
			return nil, s.errorf(cc, "case does not return a string literal")
		}
		if name == "" {
			continue
		}
		var id *ast.Ident
		if len(cc.List) == 1 {
			id, ok = cc.List[0].(*ast.Ident)
		}
		if id == nil || !ok {
			return nil, s.errorf(cc, "case does not name one Action constant")
		}
		names[id.Name] = name
	}
	return names, nil
}

func soleSwitch(body *ast.BlockStmt) (*ast.SwitchStmt, bool) {
	if len(body.List) != 1 {
		return nil, false
	}
	sw, ok := body.List[0].(*ast.SwitchStmt)
	return sw, ok
}

func clauses(sw *ast.SwitchStmt) []*ast.CaseClause {
	var out []*ast.CaseClause
	for _, st := range sw.Body.List {
		if cc, ok := st.(*ast.CaseClause); ok {
			out = append(out, cc)
		}
	}
	return out
}

func returnedString(cc *ast.CaseClause) (string, bool) {
	if len(cc.Body) != 1 {
		return "", false
	}
	ret, ok := cc.Body[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return "", false
	}
	return stringLit(ret.Results[0])
}

// ec2Dispatch lists the cases of the one `switch action` in serve() whose default clause calls
// resource.Unimplemented. A case is a string literal or resource.ActionX.String().
func ec2Dispatch(s source, names map[string]string) ([]string, error) {
	fn, err := s.funcDecl("serve", "handler")
	if err != nil {
		return nil, err
	}
	var out []string
	var dispatch *ast.SwitchStmt
	var shapeErr error
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if shapeErr != nil {
			return false
		}
		if be, ok := n.(*ast.BinaryExpr); ok && (be.Op == token.EQL || be.Op == token.NEQ) &&
			(isIdent(be.X, "action") || isIdent(be.Y, "action")) {
			shapeErr = s.errorf(be, "action is compared outside the dispatch switch")
			return false
		}
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		if !callsUnimplemented(sw) {
			if isIdent(sw.Tag, "action") {
				shapeErr = s.errorf(sw, "switch on action whose default does not call resource.Unimplemented")
				return false
			}
			return true
		}
		if dispatch != nil {
			shapeErr = s.errorf(sw, "second switch whose default calls resource.Unimplemented")
			return false
		}
		dispatch = sw
		return true
	})
	if shapeErr != nil {
		return nil, shapeErr
	}
	if dispatch == nil {
		return nil, s.errorf(fn, "no switch whose default calls resource.Unimplemented")
	}
	for _, cc := range clauses(dispatch) {
		for _, e := range cc.List {
			name, err := ec2CaseName(s, e, names)
			if err != nil {
				return nil, err
			}
			if !opNameRE.MatchString(name) {
				return nil, s.errorf(e, "dispatch case %q is not an operation name", name)
			}
			if slices.Contains(out, name) {
				return nil, s.errorf(e, "operation %s is dispatched twice", name)
			}
			out = append(out, name)
		}
	}
	return out, nil
}

func callsUnimplemented(sw *ast.SwitchStmt) bool {
	for _, cc := range clauses(sw) {
		if cc.List != nil {
			continue
		}
		found := false
		ast.Inspect(cc, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && types.ExprString(call.Fun) == "resource.Unimplemented" {
				found = true
			}
			return !found
		})
		return found
	}
	return false
}

func ec2CaseName(s source, e ast.Expr, names map[string]string) (string, error) {
	if v, ok := stringLit(e); ok {
		return v, nil
	}
	if call, ok := e.(*ast.CallExpr); ok && len(call.Args) == 0 {
		if m, ok := strings.CutPrefix(types.ExprString(call.Fun), "resource."); ok {
			if c, ok := strings.CutSuffix(m, ".String"); ok {
				if name, ok := names[c]; ok {
					return name, nil
				}
				return "", s.errorf(e, "%s is not in Action.String", c)
			}
		}
	}
	return "", s.errorf(e, "dispatch case %s is neither a string literal nor resource.Action*.String()",
		types.ExprString(e))
}

// s3Dispatch lists the operations behind the mux patterns registered in New(). It requires the
// patterns to equal the keys of s3RouteOps.
func s3Dispatch(s source) ([]string, error) {
	fn, err := s.funcDecl("New", "")
	if err != nil {
		return nil, err
	}
	routes := map[string]bool{}
	var shapeErr error
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle") {
			return true
		}
		if len(call.Args) == 0 {
			shapeErr = s.errorf(call, "%s has no pattern", sel.Sel.Name)
			return false
		}
		pattern, ok := stringLit(call.Args[0])
		if !ok || routes[pattern] {
			shapeErr = s.errorf(call, "%s pattern is not a unique string literal", sel.Sel.Name)
			return false
		}
		routes[pattern] = true
		return true
	})
	if shapeErr != nil {
		return nil, shapeErr
	}
	var out []string
	for pattern, ops := range s3RouteOps {
		if !routes[pattern] {
			return nil, s.errorf(fn, "route %q is in the S3 route table but not registered", pattern)
		}
		out = append(out, ops...)
	}
	for pattern := range routes {
		if _, ok := s3RouteOps[pattern]; !ok {
			return nil, s.errorf(fn, "route %q is registered but not in the S3 route table", pattern)
		}
	}
	return out, nil
}
