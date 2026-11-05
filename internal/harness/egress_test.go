package harness

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// symbol names one package-level object. An empty Name matches every object of Pkg, and a Pkg
// ending in "/*" matches every package under that prefix.
type symbol struct{ Pkg, Name string }

// site is a module-relative file plus its enclosing top-level declaration: a func name, "Recv.Method"
// for a method, or the first name of a package-level var, const or type spec.
type site struct{ File, Owner string }

// grant lets one site use one concrete symbol. A hit in the census has the same shape.
type grant struct {
	Site site
	Sym  symbol
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

// egressSymbols are denied in production files unless a grant names the site. Methods and struct
// fields are not package-level objects, so the table names the types and constructors that reach them.
var egressSymbols = slices.Concat(
	pkgSymbols("net",
		"Dial", "DialTimeout", "DialIP", "DialTCP", "DialUDP", "DialUnix", "Dialer",
		"Resolver", "DefaultResolver",
		"LookupAddr", "LookupCNAME", "LookupHost", "LookupIP", "LookupMX", "LookupNS", "LookupPort",
		"LookupSRV", "LookupTXT",
		"ResolveTCPAddr", "ResolveUDPAddr", "ResolveIPAddr",
		// A datagram socket sends to any address, and ListenConfig.ListenPacket is a method.
		"ListenPacket", "ListenUDP", "ListenIP", "ListenMulticastUDP", "ListenConfig"),
	pkgSymbols("crypto/tls", "Dial", "DialWithDialer", "Dialer"),
	pkgSymbols("net/http",
		"Client", "Transport", "DefaultClient", "DefaultTransport",
		"Get", "Head", "Post", "PostForm", "ProxyFromEnvironment", "ProxyURL"),
	pkgSymbols("net/http/httputil", "ReverseProxy", "NewSingleHostReverseProxy"),
	pkgSymbols("net/textproto", "Dial"),
	pkgSymbols("net/rpc"),
	pkgSymbols("net/smtp"),
	pkgSymbols("golang.org/x/net/http2"),
	pkgSymbols("golang.org/x/net/proxy"),
	pkgSymbols("golang.org/x/net/websocket"),
	pkgSymbols(awsSDK+"aws/transport/http"),
	pkgSymbols("os/exec", "Command", "CommandContext", "Cmd"),
	pkgSymbols("os", "StartProcess"),
	pkgSymbols("syscall", "Socket", "Connect", "Sendto", "Sendmsg", "ForkExec", "Exec", "StartProcess",
		"Syscall", "Syscall6", "RawSyscall", "RawSyscall6"),
	pkgSymbols("golang.org/x/sys/unix", "Socket", "Connect", "Sendto", "Sendmsg", "ForkExec", "Exec",
		"Syscall", "Syscall6", "RawSyscall", "RawSyscall6"),
	sdkEgress,
)

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
	sdkEgress,
)

// egressGrants is the whole set of exceptions. Every entry must be used by the tree
// (TestEgressGrantsAreAllUsed), so a grant outlives neither its site nor its symbol.
var egressGrants = []grant{
	{site{"internal/harness/dial.go", "lookupHost"}, symbol{"net", "DefaultResolver"}},
	{site{"internal/harness/dial.go", "DialAllowed"}, symbol{"net", "Dialer"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Client"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "DefaultTransport"}},
	{site{"internal/harness/dial.go", "NewHTTPClient"}, symbol{"net/http", "Transport"}},
	{site{"internal/harness/pin.go", "pinDial"}, symbol{"net", "Dialer"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net", "ListenConfig"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net/http", "Transport"}},
	{site{"internal/harness/pin.go", "PinEndpoint"}, symbol{"net/http/httputil", "ReverseProxy"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "config", "LoadDefaultConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/ec2", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/iam", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/s3", "NewFromConfig"}},
	{site{"internal/harness/client.go", "Load"}, symbol{awsSDK + "service/sts", "NewFromConfig"}},
	{site{"internal/engine/cli.go", "RunCLI"}, symbol{"os/exec", "CommandContext"}},
	// These tests stand in for the aws CLI, which sends through the pinned loopback proxy with a
	// default client.
	{site{"internal/harness/pin_test.go", "TestPinRefusesChangedResolution"}, symbol{"net/http", "DefaultClient"}},
	{site{"internal/harness/pin_test.go", "TestPinDialsApprovedAddress"}, symbol{"net/http", "DefaultClient"}},
	{site{"internal/harness/pin_test.go", "TestPinKeepsSignedHost"}, symbol{"net/http", "DefaultClient"}},
}

// egressCensus holds the denied symbols the module uses, one entry per site and symbol.
type egressCensus struct {
	hits       []grant
	uncompiled []string
}

func (c egressCensus) violations(grants []grant) []string {
	var out []string
	for _, h := range c.hits {
		if !slices.Contains(grants, h) {
			out = append(out, fmt.Sprintf("%s: %s: %s.%s", h.Site.File, h.Site.Owner, h.Sym.Pkg, h.Sym.Name))
		}
	}
	for _, f := range c.uncompiled {
		out = append(out, f+": not compiled under the default build")
	}
	return out
}

func (c egressCensus) unusedGrants(grants []grant) []grant {
	return slices.DeleteFunc(slices.Clone(grants), func(g grant) bool { return slices.Contains(c.hits, g) })
}

func runEgress(t *testing.T, overlay map[string][]byte) egressCensus {
	t.Helper()
	root := moduleRoot(t)
	pkgs := loadModule(t, overlay, true)
	var c egressCensus
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			rel := relPath(t, root, pkg.Fset.Position(file.Pos()).Filename)
			if seen[rel] || strings.HasPrefix(rel, "../") || slices.Contains(strings.Split(rel, "/"), "testdata") {
				continue
			}
			seen[rel] = true
			denied := egressSymbols
			if strings.HasSuffix(rel, "_test.go") {
				denied = testEgressSymbols
			}
			c.hits = append(c.hits, fileHits(pkg.TypesInfo, file, rel, denied)...)
		}
	}
	slices.SortFunc(c.hits, func(a, b grant) int {
		return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
	})
	c.hits = slices.Compact(c.hits)
	c.uncompiled = uncompiledFiles(t, root, pkgs, overlay)
	return c
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
// counts the same as the plain package name. A type used only as the operand of * is a pointer
// declaration, which creates no value, so it stays legal. A composite literal, new(T), a zero-value
// var and embedding all use the type bare and are reported.
func fileHits(info *types.Info, file *ast.File, rel string, denied []symbol) []grant {
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
	var out []grant
	for _, decl := range file.Decls {
		for _, d := range ownedDecls(decl) {
			ast.Inspect(d.node, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				obj := info.Uses[id]
				if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
					return true
				}
				if _, isType := obj.(*types.TypeName); isType && pointee[id] {
					return true
				}
				if path := obj.Pkg().Path(); anyMatch(denied, path, obj.Name()) {
					out = append(out, grant{site{rel, d.owner}, symbol{path, obj.Name()}})
				}
				return true
			})
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
`, []string{"s3/plant_dialer.go: plantDialer: net.Dialer"}},
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
`, []string{"s3/plant_exec.go: plantExec: os/exec.Command"}},
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
`, []string{"s3/plant_proxy.go: plantProxy: net/http/httputil.NewSingleHostReverseProxy"}},
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
)

func plantLegal(ctx context.Context, c *http.Client, rt http.RoundTripper) (http.Header, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1", nil)
	if err != nil {
		return nil, 0, err
	}
	if _, ok := rt.(*http.Transport); ok {
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
	}
	overlay := map[string][]byte{}
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
