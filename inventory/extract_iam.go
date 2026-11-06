package inventory

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

func readIAM(dir string) (map[API][]string, error) {
	src, err := parseSource(dir, fileIAM)
	if err != nil {
		return nil, err
	}
	funcs, err := actionTable(src)
	if err != nil {
		return nil, err
	}
	found := map[API][]string{}
	for _, api := range []API{IAM, STS} {
		if found[api], err = tableOps(src, funcs[api]); err != nil {
			return nil, err
		}
	}
	return found, nil
}

// actionTable reads actionTable(), which maps each sigv4.ServiceX to the function that builds its
// action map, and returns API -> function name. It is how the tool learns that ServiceSTS is
// served by stsActions rather than assuming it.
func actionTable(s source) (map[API]string, error) {
	fn, err := s.funcDecl("actionTable", "")
	if err != nil {
		return nil, err
	}
	lit, ok := soleReturnedLiteral(fn.Body)
	if !ok {
		return nil, s.errorf(fn, "actionTable is not a single return of a map literal")
	}
	funcs := map[API]string{}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return nil, s.errorf(e, "actionTable entry is not key: value")
		}
		svc, ok := strings.CutPrefix(types.ExprString(kv.Key), "sigv4.Service")
		api := API(strings.ToLower(svc))
		var fnName string
		if call, isCall := kv.Value.(*ast.CallExpr); isCall && len(call.Args) == 0 {
			if id, isIdent := call.Fun.(*ast.Ident); isIdent {
				fnName = id.Name
			}
		}
		if !ok || (api != IAM && api != STS) || fnName == "" {
			return nil, s.errorf(e, "actionTable entry %s is not sigv4.ServiceIAM or ServiceSTS mapped to f()",
				types.ExprString(e))
		}
		funcs[api] = fnName
	}
	if len(funcs) != 2 {
		return nil, s.errorf(lit, "actionTable has %d entries, want IAM and STS", len(funcs))
	}
	return funcs, nil
}

func soleReturnedLiteral(body *ast.BlockStmt) (*ast.CompositeLit, bool) {
	if len(body.List) != 1 {
		return nil, false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return nil, false
	}
	lit, ok := ret.Results[0].(*ast.CompositeLit)
	return lit, ok
}

// tableOps lists the keys of the action map that function name builds. The map is a literal,
// optionally returned directly, plus index assignments (m[key] = ...) at the top level of the
// function or inside a `for range <map literal of positional structs>` whose key is built from
// the loop value's string fields.
func tableOps(s source, name string) ([]string, error) {
	fn, err := s.funcDecl(name, "")
	if err != nil {
		return nil, err
	}
	var out []string
	mapVar := ""
	add := func(n ast.Node, op string) error {
		for _, have := range out {
			if have == op {
				return s.errorf(n, "action %s is defined twice", op)
			}
		}
		out = append(out, op)
		return nil
	}
	for _, st := range fn.Body.List {
		switch st := st.(type) {
		case *ast.AssignStmt:
			v, lit, ok := mapLiteral(st)
			if !ok || mapVar != "" {
				return nil, s.errorf(st, "unrecognized statement in %s", name)
			}
			mapVar = v
			if err := literalKeys(s, lit, add); err != nil {
				return nil, err
			}
		case *ast.ReturnStmt:
			if len(st.Results) != 1 {
				return nil, s.errorf(st, "unrecognized return in %s", name)
			}
			if lit, ok := st.Results[0].(*ast.CompositeLit); ok {
				if mapVar != "" {
					return nil, s.errorf(st, "second action map in %s", name)
				}
				if err := literalKeys(s, lit, add); err != nil {
					return nil, err
				}
			}
		case *ast.RangeStmt:
			if mapVar == "" {
				return nil, s.errorf(st, "loop before the action map in %s", name)
			}
			if err := loopKeys(s, st, mapVar, add); err != nil {
				return nil, err
			}
		default:
			return nil, s.errorf(st, "unrecognized statement in %s", name)
		}
	}
	if len(out) == 0 {
		return nil, s.errorf(fn, "%s defines no actions", name)
	}
	return out, nil
}

// mapLiteral matches `m := map[...]...{...}` and returns m.
func mapLiteral(st *ast.AssignStmt) (string, *ast.CompositeLit, bool) {
	if st.Tok != token.DEFINE || len(st.Lhs) != 1 || len(st.Rhs) != 1 {
		return "", nil, false
	}
	id, ok := st.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	lit, ok := st.Rhs[0].(*ast.CompositeLit)
	if !ok {
		return "", nil, false
	}
	_, isMap := lit.Type.(*ast.MapType)
	return id.Name, lit, isMap
}

func literalKeys(s source, lit *ast.CompositeLit, add func(ast.Node, string) error) error {
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return s.errorf(e, "action map entry is not key: value")
		}
		key, ok := stringLit(kv.Key)
		if !ok {
			return s.errorf(kv.Key, "action key %s is not a string literal", types.ExprString(kv.Key))
		}
		if err := add(kv.Key, key); err != nil {
			return err
		}
	}
	return nil
}

// loopKeys evaluates every `m[key] = ...` in the loop body once per element of the ranged map.
func loopKeys(s source, rs *ast.RangeStmt, mapVar string, add func(ast.Node, string) error) error {
	elems, fields, err := loopElements(s, rs)
	if err != nil {
		return err
	}
	value, ok := rs.Value.(*ast.Ident)
	if !ok {
		return s.errorf(rs, "loop has no value variable")
	}
	for _, st := range rs.Body.List {
		as, ok := st.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 {
			continue
		}
		idx, ok := as.Lhs[0].(*ast.IndexExpr)
		if !ok || types.ExprString(idx.X) != mapVar {
			continue
		}
		for _, el := range elems {
			env := map[string]string{}
			for i, f := range fields {
				if v, ok := stringLit(el[i]); ok {
					env[value.Name+"."+f] = v
				}
			}
			key, err := evalKey(s, idx.Index, env)
			if err != nil {
				return err
			}
			if err := add(idx, key); err != nil {
				return err
			}
		}
	}
	return nil
}

// loopElements returns the positional element values of the ranged map literal and the struct
// field names they bind to.
func loopElements(s source, rs *ast.RangeStmt) ([][]ast.Expr, []string, error) {
	lit, st, ok := rangedStructMap(rs)
	if !ok {
		return nil, nil, s.errorf(rs, "loop does not range over a map literal of structs")
	}
	var fields []string
	for _, f := range st.Fields.List {
		for _, n := range f.Names {
			fields = append(fields, n.Name)
		}
	}
	var elems [][]ast.Expr
	for _, e := range lit.Elts {
		vals, ok := structValues(e, len(fields))
		if !ok {
			return nil, nil, s.errorf(e, "loop element is not a positional struct literal with %d fields", len(fields))
		}
		elems = append(elems, vals)
	}
	return elems, fields, nil
}

func structValues(e ast.Expr, n int) ([]ast.Expr, bool) {
	kv, ok := e.(*ast.KeyValueExpr)
	if !ok {
		return nil, false
	}
	val, ok := kv.Value.(*ast.CompositeLit)
	if !ok || len(val.Elts) != n {
		return nil, false
	}
	return val.Elts, true
}

func rangedStructMap(rs *ast.RangeStmt) (*ast.CompositeLit, *ast.StructType, bool) {
	lit, ok := rs.X.(*ast.CompositeLit)
	if !ok {
		return nil, nil, false
	}
	mt, ok := lit.Type.(*ast.MapType)
	if !ok {
		return nil, nil, false
	}
	st, ok := mt.Value.(*ast.StructType)
	return lit, st, ok
}

// evalKey evaluates a key built from string literals, +, parentheses and loop fields in env. Any
// other node is an error quoting the expression, so a new key shape fails the tool.
func evalKey(s source, x ast.Expr, env map[string]string) (string, error) {
	switch x := x.(type) {
	case *ast.BasicLit:
		if v, ok := stringLit(x); ok {
			return v, nil
		}
	case *ast.ParenExpr:
		return evalKey(s, x.X, env)
	case *ast.SelectorExpr:
		if v, ok := env[types.ExprString(x)]; ok {
			return v, nil
		}
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			l, err := evalKey(s, x.X, env)
			if err != nil {
				return "", err
			}
			r, err := evalKey(s, x.Y, env)
			if err != nil {
				return "", err
			}
			return l + r, nil
		}
	}
	return "", s.errorf(x, "cannot evaluate action key %s", types.ExprString(x))
}
