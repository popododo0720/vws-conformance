package harness

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// symbol names one package-level object, or with a "Type.Member" Name one method or field of a
// package-level type. An empty Name matches every object of Pkg, and a Pkg ending in "/*" matches
// every package under that prefix.
type symbol struct{ Pkg, Name string }

// site is a module-relative file plus its enclosing top-level declaration: a func name, "Recv.Method"
// for a method, the first name of a package-level var, const or type spec, or "import".
type site struct{ File, Owner string }

// grant lets one site use one concrete symbol. A hit in the census has the same shape.
type grant struct {
	Site site
	Sym  symbol
}

func (s symbol) String() string {
	if s.Name == "" {
		return s.Pkg
	}
	return s.Pkg + "." + s.Name
}

func (s symbol) matches(path, name string) bool {
	if prefix, ok := strings.CutSuffix(s.Pkg, "*"); ok {
		if !strings.HasPrefix(path, prefix) {
			return false
		}
	} else if path != s.Pkg {
		return false
	}
	return s.Name == "" || s.Name == name
}

func anyMatch(syms []symbol, path, name string) bool {
	return slices.ContainsFunc(syms, func(s symbol) bool { return s.matches(path, name) })
}

// pkgSymbols names objects of one package, or the whole package when no name is given.
func pkgSymbols(pkg string, names ...string) []symbol {
	if len(names) == 0 {
		return []symbol{{Pkg: pkg}}
	}
	out := make([]symbol, len(names))
	for i, n := range names {
		out[i] = symbol{pkg, n}
	}
	return out
}

const awsSDK = "github.com/aws/aws-sdk-go-v2/"

// sdkEgress are the AWS SDK entry points that build a client, load a shared config or profile, or
// reach credential and instance-metadata sources. They are denied in test files too.
var sdkEgress = slices.Concat(
	pkgSymbols(awsSDK+"config", "LoadDefaultConfig", "LoadSharedConfigProfile"),
	pkgSymbols(awsSDK+"aws", "NewConfig"),
	pkgSymbols(awsSDK+"service/*", "New", "NewFromConfig"),
	pkgSymbols(awsSDK+"feature/ec2/imds", "New", "NewFromConfig", "Client"),
	pkgSymbols(awsSDK+"credentials/*"),
)

// guardedTypes dial, resolve, proxy or start a process through their methods and fields, and a
// nil or decoded value of one is as live as a built one. Outside a grant, production code may not
// name one, hold a value whose type contains one, or select one of its members.
var guardedTypes = slices.Concat(
	pkgSymbols("net", "Dialer", "Resolver", "ListenConfig"),
	pkgSymbols("crypto/tls", "Dialer"),
	pkgSymbols("net/http", "Client", "Transport"),
	pkgSymbols("net/http/httputil", "ReverseProxy"),
	pkgSymbols("net/http/httptest"),
	pkgSymbols("os/exec", "Cmd"),
)

// egressSymbols are denied in production files unless a grant names the site.
var egressSymbols = slices.Concat(
	guardedTypes,
	pkgSymbols("net",
		"Dial", "DialTimeout", "DialIP", "DialTCP", "DialUDP", "DialUnix", "DefaultResolver",
		"LookupAddr", "LookupCNAME", "LookupHost", "LookupIP", "LookupMX", "LookupNS", "LookupPort",
		"LookupSRV", "LookupTXT",
		"ResolveTCPAddr", "ResolveUDPAddr", "ResolveIPAddr",
		// A datagram socket sends to any address.
		"ListenPacket", "ListenUDP", "ListenIP", "ListenMulticastUDP"),
	pkgSymbols("crypto/tls", "Dial", "DialWithDialer"),
	pkgSymbols("net/http",
		"DefaultClient", "DefaultTransport", "Get", "Head", "Post", "PostForm", "ProxyFromEnvironment",
		"ProxyURL"),
	pkgSymbols("net/http/httputil", "NewSingleHostReverseProxy"),
	pkgSymbols("net/http/cgi"),
	pkgSymbols("net/http/fcgi"),
	pkgSymbols("net/textproto", "Dial"),
	pkgSymbols("net/rpc"),
	pkgSymbols("net/rpc/*"),
	pkgSymbols("net/smtp"),
	pkgSymbols("log/syslog"),
	pkgSymbols("plugin"),
	// reflect.New, Zero or Value.Convert builds a value of a guarded type from a reflect.Type
	// alone, so no expression in the file has that type.
	pkgSymbols("reflect"),
	pkgSymbols("golang.org/x/net/http2"),
	pkgSymbols("golang.org/x/net/proxy"),
	pkgSymbols("golang.org/x/net/websocket"),
	pkgSymbols(awsSDK+"aws/transport/http"),
	pkgSymbols("os/exec", "Command", "CommandContext"),
	pkgSymbols("os", "StartProcess"),
	pkgSymbols("syscall", "Socket", "Connect", "Sendto", "Sendmsg", "ForkExec", "Exec", "StartProcess",
		"Syscall", "Syscall6", "RawSyscall", "RawSyscall6"),
	pkgSymbols("golang.org/x/sys/unix", "Socket", "Connect", "Sendto", "Sendmsg", "ForkExec", "Exec",
		"Syscall", "Syscall6", "RawSyscall", "RawSyscall6"),
	sdkEgress,
)

// deniedImports may not be imported by a production file at all. unsafe reaches unexported fields
// and enables go:linkname, and cgo calls C, all out of sight of the type checker the census reads.
var deniedImports = []string{"unsafe", "C"}

// testEgressSymbols are denied in _test.go files. A test reaches its loopback fake through the
// httptest server's own client or a harness client, never a default or hand-built one.
var testEgressSymbols = slices.Concat(
	pkgSymbols("net",
		"Dial", "DialTimeout", "DialIP", "DialTCP", "DialUDP", "DialUnix", "Dialer",
		"Resolver", "DefaultResolver",
		"LookupAddr", "LookupCNAME", "LookupHost", "LookupIP", "LookupMX", "LookupNS", "LookupPort",
		"LookupSRV", "LookupTXT"),
	pkgSymbols("crypto/tls", "Dial", "DialWithDialer", "Dialer"),
	pkgSymbols("net/http",
		"Client", "Transport", "DefaultClient", "DefaultTransport",
		"Get", "Head", "Post", "PostForm", "ProxyFromEnvironment", "ProxyURL"),
	pkgSymbols("net/http/httputil", "NewSingleHostReverseProxy", "ReverseProxy"),
	pkgSymbols("net/http/cgi"),
	pkgSymbols("net/rpc"),
	pkgSymbols("net/rpc/*"),
	pkgSymbols("log/syslog"),
	pkgSymbols("os", "StartProcess"),
	sdkEgress,
)

// egressGrants is the whole set of exceptions. Every entry must be used by the tree
// (TestEgressGrantsAreAllUsed), so a grant outlives neither its site nor its symbol.
var egressGrants = []grant{
	{site{"internal/harness/dial.go", "DialAllowed"}, symbol{"net", "Dialer"}},
	{site{"internal/harness/dial.go", "DialAllowed"}, symbol{"net", "Dialer.DialContext"}},
	{site{"internal/harness/dial.go", "GuardedClient.Do"}, symbol{"net/http", "Client"}},
	{site{"internal/harness/dial.go", "GuardedClient.Do"}, symbol{"net/http", "Client.Do"}},
	{site{"internal/harness/dial.go", "GuardedClient"}, symbol{"net/http", "Client"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Client"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "DefaultTransport"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Transport"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Transport.Clone"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Transport.DialContext"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Transport.Proxy"}},
	{site{"internal/harness/dial.go", "lookupHost"}, symbol{"net", "DefaultResolver"}},
	{site{"internal/harness/dial.go", "lookupHost"}, symbol{"net", "Resolver"}},
	{site{"internal/harness/dial.go", "lookupHost"}, symbol{"net", "Resolver.LookupIP"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net", "ListenConfig"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net", "ListenConfig.Listen"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net/http", "Transport"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net/http", "Transport.TLSClientConfig"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net/http/httputil", "ReverseProxy"}},
	{site{"internal/harness/pin.go", "pinDial"}, symbol{"net", "Dialer"}},
	{site{"internal/harness/pin.go", "pinDial"}, symbol{"net", "Dialer.DialContext"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "config", "LoadDefaultConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/ec2", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/iam", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/s3", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/sts", "NewFromConfig"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "Cmd"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "Cmd.Env"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "Cmd.Run"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "Cmd.Stderr"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "Cmd.Stdout"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "CommandContext"}},
	// ec2fake is the loopback EC2 server the tests run against.
	{site{"internal/ec2fake/ec2fake.go", "New"}, symbol{"net/http/httptest", "NewServer"}},
	{site{"internal/ec2fake/ec2fake.go", "New"}, symbol{"net/http/httptest", "Server"}},
	{site{"internal/ec2fake/ec2fake.go", "Server"}, symbol{"net/http/httptest", "Server"}},
	// These tests stand in for the aws CLI, which sends through the pinned loopback proxy with a
	// default client.
	{site{"internal/harness/pin_test.go", "TestPinDialsApprovedAddress"}, symbol{"net/http", "DefaultClient"}},
	{site{"internal/harness/pin_test.go", "TestPinKeepsSignedHost"}, symbol{"net/http", "DefaultClient"}},
	{site{"internal/harness/pin_test.go", "TestPinRefusesChangedResolution"}, symbol{"net/http", "DefaultClient"}},
}

// egressCensus holds the denied symbols the module uses, one entry per site and symbol, and the
// files the census cannot read.
type egressCensus struct {
	hits   []grant
	unread []string
}

func (c egressCensus) violations(grants []grant) []string {
	var out []string
	for _, h := range c.hits {
		if !slices.Contains(grants, h) {
			out = append(out, fmt.Sprintf("%s: %s: %s", h.Site.File, h.Site.Owner, h.Sym))
		}
	}
	return append(out, c.unread...)
}

func (c egressCensus) unusedGrants(grants []grant) []grant {
	return slices.DeleteFunc(slices.Clone(grants), func(g grant) bool { return slices.Contains(c.hits, g) })
}

func runEgress(t *testing.T, overlay map[string][]byte) egressCensus {
	t.Helper()
	root := moduleRoot(t)
	pkgs := loadModule(t, overlay, true)
	loaded := map[string]bool{}
	module := ""
	for _, pkg := range pkgs {
		loaded[pkg.PkgPath] = true
		if pkg.Module != nil {
			module = pkg.Module.Path
		}
	}
	if module == "" {
		t.Fatal("no loaded package names its module")
	}
	var c egressCensus
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.OtherFiles {
			if rel := relPath(t, root, f); !strings.HasPrefix(rel, "../") {
				c.unread = append(c.unread, rel+": not Go source")
			}
		}
		for _, file := range pkg.Syntax {
			rel := relPath(t, root, pkg.Fset.Position(file.Pos()).Filename)
			if seen[rel] || strings.HasPrefix(rel, "../") {
				continue
			}
			seen[rel] = true
			c.unread = append(c.unread, unloadedImports(file, rel, module, loaded)...)
			c.hits = append(c.hits, fileHits(pkg.TypesInfo, file, rel, strings.HasSuffix(rel, "_test.go"))...)
		}
	}
	slices.SortFunc(c.hits, func(a, b grant) int {
		return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})
	c.hits = slices.Compact(c.hits)
	for _, f := range uncompiledFiles(t, root, pkgs, overlay) {
		c.unread = append(c.unread, f+": not compiled under the default build")
	}
	return c
}

// unloadedImports reports imports of module packages that ./... does not load, such as a package
// under testdata, so the census never reads them.
func unloadedImports(file *ast.File, rel, module string, loaded map[string]bool) []string {
	var out []string
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err == nil && (path == module || strings.HasPrefix(path, module+"/")) && !loaded[path] {
			out = append(out, fmt.Sprintf("%s: imports %s, which ./... does not load", rel, path))
		}
	}
	return out
}

type ownedDecl struct {
	node  ast.Node
	owner string
}

func ownedDecls(decl ast.Decl) []ownedDecl {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		owner := d.Name.Name
		if d.Recv != nil && len(d.Recv.List) == 1 {
			recv := ast.Unparen(d.Recv.List[0].Type)
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = ast.Unparen(star.X)
			}
			switch r := recv.(type) {
			case *ast.IndexExpr:
				recv = r.X
			case *ast.IndexListExpr:
				recv = r.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				owner = id.Name + "." + owner
			}
		}
		return []ownedDecl{{d, owner}}
	case *ast.GenDecl:
		var out []ownedDecl
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				out = append(out, ownedDecl{s, s.Names[0].Name})
			case *ast.TypeSpec:
				out = append(out, ownedDecl{s, s.Name.Name})
			}
		}
		return out
	}
	return nil
}

// fileHits resolves every identifier through the type checker, so an import alias or a dot import
// counts the same as the plain package name. In a production file it also reports every member
// selected from a guarded type and every expression whose type contains one, so a value that
// arrives through decoding, generics or inference is seen without its type being named.
func fileHits(info *types.Info, file *ast.File, rel string, test bool) []grant {
	var out []grant
	add := func(owner string, s symbol) { out = append(out, grant{site{rel, owner}, s}) }
	denied, pointee := egressSymbols, map[*ast.Ident]bool{}
	if test {
		denied, pointee = testEgressSymbols, pointeeTypes(file)
	} else {
		for _, imp := range file.Imports {
			if path, err := strconv.Unquote(imp.Path.Value); err == nil && slices.Contains(deniedImports, path) {
				add("import", symbol{Pkg: path})
			}
		}
	}
	for _, decl := range file.Decls {
		for _, d := range ownedDecls(decl) {
			ast.Inspect(d.node, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if s, ok := deniedUse(info, id, denied, pointee); ok {
						add(d.owner, s)
					}
				}
				if test {
					return true
				}
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if s, ok := guardedMember(info.Selections[sel]); ok {
						add(d.owner, s)
					}
				}
				if e, ok := n.(ast.Expr); ok {
					for _, s := range guardedIn(info.TypeOf(e), nil) {
						add(d.owner, s)
					}
				}
				return true
			})
		}
	}
	return out
}

// pointeeTypes are the identifiers used as the operand of *. In a test file a pointer declaration
// creates no value, so it stays legal there. A composite literal, new(T), a zero-value var and
// embedding all use the type bare and are reported.
func pointeeTypes(file *ast.File) map[*ast.Ident]bool {
	pointee := map[*ast.Ident]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if star, ok := n.(*ast.StarExpr); ok {
			switch x := ast.Unparen(star.X).(type) {
			case *ast.Ident:
				pointee[x] = true
			case *ast.SelectorExpr:
				pointee[x.Sel] = true
			}
		}
		return true
	})
	return pointee
}

func deniedUse(info *types.Info, id *ast.Ident, denied []symbol, pointee map[*ast.Ident]bool) (symbol, bool) {
	obj := info.Uses[id]
	if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return symbol{}, false
	}
	if _, isType := obj.(*types.TypeName); isType && pointee[id] {
		return symbol{}, false
	}
	path := obj.Pkg().Path()
	return symbol{path, obj.Name()}, anyMatch(denied, path, obj.Name())
}

// guardedMember reports a method or field selection whose owner is a guarded type. The owner is the
// type that declares the member, found by walking the embedding path, so a member promoted
// through a wrapper struct still counts.
func guardedMember(sel *types.Selection) (symbol, bool) {
	if sel == nil {
		return symbol{}, false
	}
	owner := sel.Recv()
	path := sel.Index()
	for _, i := range path[:len(path)-1] {
		st, ok := deref(owner).Underlying().(*types.Struct)
		if !ok {
			return symbol{}, false
		}
		owner = st.Field(i).Type()
	}
	s, ok := guardedName(deref(owner))
	s.Name += "." + sel.Obj().Name()
	return s, ok
}

func deref(t types.Type) types.Type {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

func guardedName(t types.Type) (symbol, bool) {
	named, ok := types.Unalias(t).(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return symbol{}, false
	}
	s := symbol{named.Obj().Pkg().Path(), named.Obj().Name()}
	return s, anyMatch(guardedTypes, s.Pkg, s.Name)
}

// guardedIn appends every guarded type that t is built from without passing through another named
// type: pointers, slices, arrays, maps, channels, function signatures, tuples and literal structs.
func guardedIn(t types.Type, out []symbol) []symbol {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		if s, ok := guardedName(t); ok {
			out = append(out, s)
		}
	case *types.Pointer:
		out = guardedIn(t.Elem(), out)
	case *types.Slice:
		out = guardedIn(t.Elem(), out)
	case *types.Array:
		out = guardedIn(t.Elem(), out)
	case *types.Chan:
		out = guardedIn(t.Elem(), out)
	case *types.Map:
		out = guardedIn(t.Elem(), guardedIn(t.Key(), out))
	case *types.Signature:
		out = guardedIn(t.Results(), guardedIn(t.Params(), out))
	case *types.Tuple:
		for v := range t.Variables() {
			out = guardedIn(v.Type(), out)
		}
	case *types.Struct:
		for f := range t.Fields() {
			out = guardedIn(f.Type(), out)
		}
	}
	return out
}

func TestEgressStaysBehindTheGuard(t *testing.T) {
	if bad := runEgress(t, nil).violations(egressGrants); len(bad) > 0 {
		t.Fatalf("network or process egress outside the guarded dialer:\n%s", strings.Join(bad, "\n"))
	}
}

func TestEgressGrantsAreAllUsed(t *testing.T) {
	if stale := runEgress(t, nil).unusedGrants(egressGrants); len(stale) > 0 {
		t.Fatalf("grants no site uses any more, remove them:\n%v", stale)
	}
}

// bypassCase plants one file in the module and lists the lines the census must report for it.
type bypassCase struct {
	name string
	file string
	body string
	want []string
}

func TestEgressCensusReportsBypass(t *testing.T) {
	root := moduleRoot(t)
	dial, err := os.ReadFile(filepath.Join(root, "internal", "harness", "dial.go"))
	if err != nil {
		t.Fatal(err)
	}
	const awsHTTPTransport = awsSDK + "aws/transport/http"
	tests := []bypassCase{
		{"http.Get in a service package", "s3/plant_get.go", `package s3

import "net/http"

func plantGet() { _, _ = http.Get("http://127.0.0.1:1") }
`, []string{"s3/plant_get.go: plantGet: net/http.Get"}},
		{"http.Client literal", "s3/plant_client.go", `package s3

import "net/http"

func plantClient() *http.Client { return &http.Client{} }
`, []string{"s3/plant_client.go: plantClient: net/http.Client"}},
		{"zero-value http.Client", "s3/plant_zero.go", `package s3

import "net/http"

var plantZero http.Client
`, []string{"s3/plant_zero.go: plantZero: net/http.Client"}},
		{"new(http.Transport)", "s3/plant_new.go", `package s3

import "net/http"

func plantNew() *http.Transport { return new(http.Transport) }
`, []string{"s3/plant_new.go: plantNew: net/http.Transport"}},
		{"embedded http.Client", "s3/plant_embed.go", `package s3

import "net/http"

type plantEmbed struct{ http.Client }
`, []string{"s3/plant_embed.go: plantEmbed: net/http.Client"}},
		{"alias import", "s3/plant_alias.go", `package s3

import h "net/http"

func plantAlias() { _, _ = h.Get("http://127.0.0.1:1") }
`, []string{"s3/plant_alias.go: plantAlias: net/http.Get"}},
		{"dot import", "s3/plant_dot.go", `package s3

import . "net/http"

func plantDot() { _, _ = PostForm("http://127.0.0.1:1", nil) }
`, []string{"s3/plant_dot.go: plantDot: net/http.PostForm"}},
		{"net.Dialer", "s3/plant_dialer.go", `package s3

import (
	"context"
	"net"
)

func plantDialer(ctx context.Context) {
	_, _ = (&net.Dialer{}).DialContext(ctx, "tcp", "127.0.0.1:1")
}
`, []string{
			"s3/plant_dialer.go: plantDialer: net.Dialer.DialContext",
			"s3/plant_dialer.go: plantDialer: net.Dialer",
		}},
		{"net.LookupIP", "s3/plant_lookup.go", `package s3

import "net"

func plantLookup() { _, _ = net.LookupIP("example.invalid") }
`, []string{"s3/plant_lookup.go: plantLookup: net.LookupIP"}},
		{"net.ListenUDP", "s3/plant_udp.go", `package s3

import "net"

func plantUDP() { _, _ = net.ListenUDP("udp", nil) }
`, []string{"s3/plant_udp.go: plantUDP: net.ListenUDP"}},
		{"exec.Command", "s3/plant_exec.go", `package s3

import "os/exec"

func plantExec() { _ = exec.Command("true") }
`, []string{
			"s3/plant_exec.go: plantExec: os/exec.Cmd",
			"s3/plant_exec.go: plantExec: os/exec.Command",
		}},
		{"syscall.Socket", "s3/plant_syscall.go", `package s3

import "syscall"

func plantSyscall() { _, _ = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0) }
`, []string{"s3/plant_syscall.go: plantSyscall: syscall.Socket"}},
		{"httputil reverse proxy", "s3/plant_proxy.go", `package s3

import (
	"net/http/httputil"
	"net/url"
)

func plantProxy(u *url.URL) { _ = httputil.NewSingleHostReverseProxy(u) }
`, []string{
			"s3/plant_proxy.go: plantProxy: net/http/httputil.NewSingleHostReverseProxy",
			"s3/plant_proxy.go: plantProxy: net/http/httputil.ReverseProxy",
		}},
		{"AWS transport package", "s3/plant_awshttp.go", `package s3

import awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"

func plantAWSHTTP() { _ = awshttp.NewBuildableClient() }
`, []string{"s3/plant_awshttp.go: plantAWSHTTP: " + awsHTTPTransport + ".NewBuildableClient"}},
		{"config.LoadDefaultConfig outside client.go", "s3/plant_config.go", `package s3

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
)

func plantConfig(ctx context.Context) { _, _ = config.LoadDefaultConfig(ctx) }
`, []string{"s3/plant_config.go: plantConfig: " + awsSDK + "config.LoadDefaultConfig"}},
		{"s3.NewFromConfig outside Load", "s3/plant_service.go", `package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func plantService(cfg aws.Config) { _ = awss3.NewFromConfig(cfg) }
`, []string{"s3/plant_service.go: plantService: " + awsSDK + "service/s3.NewFromConfig"}},
		{"sso service constructor", "s3/plant_sso.go", `package s3

import "github.com/aws/aws-sdk-go-v2/service/sso"

func plantSSO() { _ = sso.New(sso.Options{}) }
`, []string{"s3/plant_sso.go: plantSSO: " + awsSDK + "service/sso.New"}},
		{"ssocreds package", "s3/plant_ssocreds.go", `package s3

import "github.com/aws/aws-sdk-go-v2/credentials/ssocreds"

func plantSSOCreds() { _ = ssocreds.New }
`, []string{"s3/plant_ssocreds.go: plantSSOCreds: " + awsSDK + "credentials/ssocreds.New"}},
		{"imds.New", "s3/plant_imds.go", `package s3

import "github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

func plantIMDS() { _ = imds.New(imds.Options{}) }
`, []string{"s3/plant_imds.go: plantIMDS: " + awsSDK + "feature/ec2/imds.New"}},
		{"new file beside dial.go", "internal/harness/dial_extra.go", `package harness

import "net/http"

func extraGet() { _, _ = http.Get("http://127.0.0.1:1") }
`, []string{"internal/harness/dial_extra.go: extraGet: net/http.Get"}},
		{"granted symbol in a new function of dial.go", "internal/harness/dial.go", string(dial) + `
func extraDial() { _ = &net.Dialer{} }
`, []string{"internal/harness/dial.go: extraDial: net.Dialer"}},
		{"test file loading the default config", "internal/harness/plant_config_test.go", `package harness

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
)

func TestPlantConfig(t *testing.T) { _, _ = config.LoadDefaultConfig(context.Background()) }
`, []string{"internal/harness/plant_config_test.go: TestPlantConfig: " + awsSDK + "config.LoadDefaultConfig"}},
		{"file behind a build tag", "s3/plant_tagged.go", `//go:build census

package s3

import "net/http"

func plantTagged() { _, _ = http.Get("http://127.0.0.1:1") }
`, []string{"s3/plant_tagged.go: not compiled under the default build"}},
		{"default client in a test file", "internal/harness/plant_default_test.go", `package harness

import (
	"net/http"
	"testing"
)

func TestPlantDefault(t *testing.T) {
	c := &http.Client{}
	_, _ = c.Get("http://127.0.0.1:1")
	_, _ = http.Get("http://127.0.0.1:1")
}
`, []string{
			"internal/harness/plant_default_test.go: TestPlantDefault: net/http.Client",
			"internal/harness/plant_default_test.go: TestPlantDefault: net/http.Get",
		}},
		{"httptest client in a test file", "internal/harness/plant_httptest_test.go", `package harness

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
)

func TestPlantHTTPTest(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, _ = srv.Client().Get(srv.URL)
	_ = exec.Command("true")
}
`, nil},
		{"legal net/http use", "s3/plant_legal.go", `package s3

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"

	"github.com/popododo0720/vws-conformance/internal/harness"
)

func plantLegal(ctx context.Context, c *harness.GuardedClient, rt http.RoundTripper) (http.Header, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1", nil)
	if err != nil {
		return nil, 0, err
	}
	if rt != nil {
		_ = imds.ClientDisabled
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	return resp.Header, http.StatusOK, nil
}
`, nil},
		{"json.Unmarshal into a nil *http.Client and *exec.Cmd", "s3/plant_unmarshal.go", `package s3

import (
	"encoding/json"
	"net/http"
	"os/exec"
)

func plantUnmarshal() {
	var c *http.Client
	_ = json.Unmarshal([]byte("{}"), &c)
	_, _ = c.Get("http://127.0.0.1:1")
	var cmd *exec.Cmd
	_ = json.Unmarshal([]byte("{}"), &cmd)
	_ = cmd.Run()
}
`, []string{
			"s3/plant_unmarshal.go: plantUnmarshal: net/http.Client.Get",
			"s3/plant_unmarshal.go: plantUnmarshal: net/http.Client",
			"s3/plant_unmarshal.go: plantUnmarshal: os/exec.Cmd.Run",
			"s3/plant_unmarshal.go: plantUnmarshal: os/exec.Cmd",
		}},
		{"generic allocator", "s3/plant_generic.go", `package s3

import "net/http"

func alloc[P interface{ *E }, E any]() P { return new(E) }

func plantGeneric() { _, _ = alloc[*http.Client]().Get("http://127.0.0.1:1") }
`, []string{
			"s3/plant_generic.go: plantGeneric: net/http.Client.Get",
			"s3/plant_generic.go: plantGeneric: net/http.Client",
		}},
		{"reflect.New of a guarded type", "s3/plant_reflect.go", `package s3

import (
	"net/http"
	"reflect"
)

func plantReflect() {
	c := reflect.New(reflect.TypeFor[*http.Client]().Elem()).Interface().(*http.Client)
	_, _ = c.Get("http://127.0.0.1:1")
}
`, []string{
			"s3/plant_reflect.go: plantReflect: net/http.Client.Get",
			"s3/plant_reflect.go: plantReflect: net/http.Client",
			"s3/plant_reflect.go: plantReflect: reflect.New",
			"s3/plant_reflect.go: plantReflect: reflect.TypeFor",
		}},
		{"nil *net.Resolver", "s3/plant_resolver.go", `package s3

import (
	"context"
	"net"
)

func plantResolver(ctx context.Context) {
	var r *net.Resolver
	_, _ = r.LookupHost(ctx, "example.invalid")
}
`, []string{
			"s3/plant_resolver.go: plantResolver: net.Resolver.LookupHost",
			"s3/plant_resolver.go: plantResolver: net.Resolver",
		}},
		{"guard stripped from a GuardedClient", "internal/harness/plant_strip.go", `package harness

import "net/http"

func plantStrip(g *GuardedClient, req *http.Request) {
	c := *g.c
	tr := c.Transport.(*http.Transport).Clone()
	tr.DialContext = nil
	c.Transport = tr
	_, _ = c.Do(req)
}
`, []string{
			"internal/harness/plant_strip.go: plantStrip: net/http.Client.Do",
			"internal/harness/plant_strip.go: plantStrip: net/http.Client.Transport",
			"internal/harness/plant_strip.go: plantStrip: net/http.Client",
			"internal/harness/plant_strip.go: plantStrip: net/http.Transport.Clone",
			"internal/harness/plant_strip.go: plantStrip: net/http.Transport.DialContext",
			"internal/harness/plant_strip.go: plantStrip: net/http.Transport",
		}},
		{"httptest client in production", "s3/plant_httptest.go", `package s3

import (
	"net/http"
	"net/http/httptest"
)

func plantHTTPTest(h http.Handler) { _, _ = httptest.NewServer(h).Client().Get("http://127.0.0.1:1") }
`, []string{
			"s3/plant_httptest.go: plantHTTPTest: net/http.Client.Get",
			"s3/plant_httptest.go: plantHTTPTest: net/http.Client",
			"s3/plant_httptest.go: plantHTTPTest: net/http/httptest.NewServer",
			"s3/plant_httptest.go: plantHTTPTest: net/http/httptest.Server.Client",
			"s3/plant_httptest.go: plantHTTPTest: net/http/httptest.Server",
		}},
		{"syslog, jsonrpc, cgi, fcgi and plugin", "s3/plant_entry.go", `package s3

import (
	"log/syslog"
	"net"
	"net/http/cgi"
	"net/http/fcgi"
	"net/rpc/jsonrpc"
	"plugin"
)

func plantEntry(l net.Listener) {
	_, _ = syslog.Dial("tcp", "127.0.0.1:1", syslog.LOG_INFO, "plant")
	_, _ = jsonrpc.Dial("tcp", "127.0.0.1:1")
	_ = &cgi.Handler{Path: "/bin/true"}
	_ = fcgi.Serve(l, nil)
	_, _ = plugin.Open("plant.so")
}
`, []string{
			"s3/plant_entry.go: plantEntry: log/syslog.Dial",
			"s3/plant_entry.go: plantEntry: log/syslog.LOG_INFO",
			"s3/plant_entry.go: plantEntry: net/http/cgi.Handler",
			"s3/plant_entry.go: plantEntry: net/http/fcgi.Serve",
			"s3/plant_entry.go: plantEntry: net/rpc/jsonrpc.Dial",
			"s3/plant_entry.go: plantEntry: plugin.Open",
		}},
		{"proxy, rpc, syslog, cgi and StartProcess in a test file", "internal/harness/plant_entry_test.go", `package harness

import (
	"log/syslog"
	"net/http/cgi"
	"net/http/httputil"
	"net/rpc"
	"net/rpc/jsonrpc"
	"net/url"
	"os"
	"testing"
)

func TestPlantEntry(t *testing.T) {
	_ = httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "127.0.0.1:1"})
	_ = &httputil.ReverseProxy{}
	_, _ = rpc.Dial("tcp", "127.0.0.1:1")
	_, _ = jsonrpc.Dial("tcp", "127.0.0.1:1")
	_, _ = syslog.Dial("tcp", "127.0.0.1:1", syslog.LOG_INFO, "plant")
	_ = &cgi.Handler{Path: "/bin/true"}
	_, _ = os.StartProcess("/bin/true", nil, &os.ProcAttr{})
}
`, []string{
			"internal/harness/plant_entry_test.go: TestPlantEntry: log/syslog.Dial",
			"internal/harness/plant_entry_test.go: TestPlantEntry: log/syslog.LOG_INFO",
			"internal/harness/plant_entry_test.go: TestPlantEntry: net/http/cgi.Handler",
			"internal/harness/plant_entry_test.go: TestPlantEntry: net/http/httputil.NewSingleHostReverseProxy",
			"internal/harness/plant_entry_test.go: TestPlantEntry: net/http/httputil.ReverseProxy",
			"internal/harness/plant_entry_test.go: TestPlantEntry: net/rpc.Dial",
			"internal/harness/plant_entry_test.go: TestPlantEntry: net/rpc/jsonrpc.Dial",
			"internal/harness/plant_entry_test.go: TestPlantEntry: os.StartProcess",
		}},
		{"import of a testdata package", "s3/plant_testdata.go", `package s3

import "github.com/popododo0720/vws-conformance/s3/testdata/leak"

func plantTestdata() { leak.Get() }
`, []string{
			"s3/plant_testdata.go: imports github.com/popododo0720/vws-conformance/s3/testdata/leak, which ./... does not load",
		}},
		{"go:linkname through unsafe", "s3/plant_linkname.go", `package s3

import _ "unsafe"

//go:linkname plantNanotime runtime.nanotime
func plantNanotime() int64
`, []string{"s3/plant_linkname.go: import: unsafe"}},
		{"assembly file", "s3/plant_asm.s", "", []string{"s3/plant_asm.s: not Go source"}},
	}
	// support files back a case. They report nothing themselves.
	support := map[string]string{
		"s3/testdata/leak/leak.go": `package leak

import "net/http"

func Get() { _, _ = http.Get("http://127.0.0.1:1") }
`,
	}
	overlay := map[string][]byte{}
	for f, body := range support {
		overlay[filepath.Join(root, filepath.FromSlash(f))] = []byte(body)
	}
	for _, tt := range tests {
		overlay[filepath.Join(root, filepath.FromSlash(tt.file))] = []byte(tt.body)
	}
	// Each case plants its own file, so one load serves them all and a line is attributed by file.
	got := runEgress(t, overlay).violations(egressGrants)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mine []string
			for _, line := range got {
				if strings.HasPrefix(line, tt.file+":") {
					mine = append(mine, line)
				}
			}
			if !slices.Equal(mine, tt.want) {
				t.Fatalf("reported:\n%s\nwant:\n%s", strings.Join(mine, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
	for _, line := range got {
		if !slices.ContainsFunc(tests, func(tt bypassCase) bool { return strings.HasPrefix(line, tt.file+":") }) {
			t.Errorf("reported a line no case planted: %s", line)
		}
	}
}
