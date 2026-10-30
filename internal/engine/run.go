package engine

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/popododo0720/vws-conformance/auth"
	"github.com/popododo0720/vws-conformance/ebs"
	"github.com/popododo0720/vws-conformance/ec2"
	"github.com/popododo0720/vws-conformance/iam"
	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/expect"
	"github.com/popododo0720/vws-conformance/internal/harness"
	"github.com/popododo0720/vws-conformance/internal/report"
	"github.com/popododo0720/vws-conformance/s3"
	"github.com/popododo0720/vws-conformance/sts"
	"github.com/popododo0720/vws-conformance/vpc"
)

var (
	flagService = flag.String("service", "", "service to run")
	flagGroup   = flag.String("group", "", "operation group to run")
	flagFull    = flag.Bool("full", false, "run every group")
)

func Groups() []bind.Group {
	return []bind.Group{
		s3.Buckets(), s3.Listings(),
		ec2.Instances(), ec2.Images(), ec2.KeyPairs(), ec2.Addresses(), ec2.Gaps(),
		vpc.Vpcs(), vpc.Subnets(), vpc.SecurityGroups(), vpc.Gaps(),
		ebs.Volumes(), ebs.Snapshots(), ebs.Gaps(),
		iam.Users(), sts.Caller(), auth.SigV4(),
	}
}

func TestGroup(t Tester, service, group string) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Selected(service, group) {
		t.Skip("not selected")
	}
	rep, err := runGroups(harness.ShutdownContext(), cfg, service, group)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.OutDir, cfg.RunID)
	if rep.Summary.Error != 0 {
		t.Fatalf("harness errors %d in %s", rep.Summary.Error, dir)
	}
	untracked := parityGate(rep, func(msg string) { t.Logf("%s", msg) })
	if len(untracked) != 0 {
		t.Fatalf("untracked parity failures %d in %s: %s", len(untracked), dir, resultIDs(untracked))
	}
}

type Tester interface {
	Fatal(args ...any)
	Skip(args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

func parityGate(rep *report.Report, log func(string)) []report.Result {
	var untracked []report.Result
	for _, res := range rep.Results {
		if res.Status != "fail" {
			continue
		}
		if res.Issue == "" {
			untracked = append(untracked, res)
			continue
		}
		log(fmt.Sprintf("known parity failure %s tracked by %s", res.ID, res.Issue))
	}
	return untracked
}

func resultIDs(results []report.Result) string {
	ids := make([]string, len(results))
	for i, res := range results {
		ids[i] = res.ID
	}
	return strings.Join(ids, ", ")
}

func Main() int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if !cfg.Full && cfg.Service == "" && cfg.Group == "" {
		fmt.Fprintln(os.Stderr, "set --service and --group, or --full")
		return 2
	}
	rep, err := runGroups(harness.ShutdownContext(), cfg, "", "")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if rep.Summary.Error != 0 {
		fmt.Fprintf(os.Stderr, "harness errors %d\n", rep.Summary.Error)
		return 1
	}
	untracked := parityGate(rep, func(msg string) { fmt.Fprintln(os.Stderr, msg) })
	if len(untracked) != 0 {
		fmt.Fprintf(os.Stderr, "untracked parity failures %d: %s\n", len(untracked), resultIDs(untracked))
		return 1
	}
	fmt.Printf("pass %d fail %d skip %d error %d\n", rep.Summary.Pass, rep.Summary.Fail, rep.Summary.Skip,
		rep.Summary.Error)
	return 0
}

func loadConfig() (harness.Config, error) {
	cfg, err := harness.FromEnv()
	if err != nil {
		return harness.Config{}, wrapErr("loadConfig", err)
	}
	if *flagService != "" {
		cfg.Service = *flagService
	}
	if *flagGroup != "" {
		cfg.Group = *flagGroup
	}
	if *flagFull {
		cfg.Full = true
	}
	if cfg.RunID == "" {
		id, err := processRunID()
		if err != nil {
			return harness.Config{}, err
		}
		cfg.RunID = id
	}
	if err := harness.ValidateRunID(cfg.RunID); err != nil {
		return harness.Config{}, wrapErr("loadConfig", err)
	}
	return cfg, nil
}

var gate struct {
	once sync.Once
	rt   *harness.Runtime
	err  error
}

var minted struct {
	once sync.Once
	id   string
	err  error
}

func processRunID() (string, error) {
	minted.once.Do(func() {
		minted.id, minted.err = harness.NewRunID(time.Now())
	})
	return minted.id, minted.err
}

func runtime(ctx context.Context, cfg harness.Config) (*harness.Runtime, error) {
	gate.once.Do(func() {
		if err := cfg.CheckEndpoints(); err != nil {
			gate.err = err
			return
		}
		cl, err := harness.Load(ctx, cfg)
		if err != nil {
			gate.err = err
			return
		}
		rt := harness.NewRuntime(cfg, cl)
		if err := harness.ClaimRun(cfg.RunID); err != nil {
			gate.err = err
			return
		}
		claimed := true
		defer func() {
			if claimed {
				if err := harness.ReleaseRun(cfg.RunID); err != nil && gate.err == nil {
					gate.err = fmt.Errorf("release run: %w", err)
				}
			}
		}()
		account, err := rt.Identify(ctx)
		if err != nil {
			gate.err = err
			return
		}
		if err := harness.CheckAccount(account, cfg.AllowedAccounts); err != nil {
			gate.err = err
			return
		}
		if err := rt.BindAccount(account); err != nil {
			gate.err = err
			return
		}
		//nolint:contextcheck // process cleanup runs after the run context is finished
		harness.SetProcessCleanup(func() error {
			if err := harness.ReleaseRun(rt.Cfg.RunID); err != nil {
				return noteCleanup(err)
			}
			return nil
		})
		claimed = false
		gate.rt = rt
	})
	return gate.rt, gate.err
}

func runGroups(ctx context.Context, cfg harness.Config, onlyService, onlyGroup string) (*report.Report, error) {
	var selected []bind.Group
	for _, g := range Groups() {
		if onlyService != "" && (g.Service != onlyService || g.Name != onlyGroup) {
			continue
		}
		if onlyService == "" && !cfg.Selected(g.Service, g.Name) {
			continue
		}
		selected = append(selected, g)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no groups selected")
	}
	known, err := parseKnownIssues(knownIssuesFile, Groups())
	if err != nil {
		return nil, err
	}
	needsLive := false
	for _, g := range selected {
		for _, c := range g.Cases {
			if c.Skip == "" {
				needsLive = true
			}
		}
	}
	var rt *harness.Runtime
	if needsLive {
		rt, err = runtime(ctx, cfg)
		if err != nil {
			return nil, err
		}
	} else {
		rt = harness.NewRuntime(cfg, nil)
	}
	rep := report.New(cfg.RunID, report.Target{
		Name: cfg.Target, Region: cfg.Region, ConsoleAPIVersion: cfg.ConsoleAPIVersion,
	}, tools())
	sem := make(chan struct{}, cfg.Parallel)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, g := range selected {
		for _, c := range g.Cases {
			wg.Add(1)
			go func(g bind.Group, c cases.Case) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				res := runOne(ctx, rt, g, c)
				if res.Status == "fail" {
					res.Issue = known[res.ID]
				}
				mu.Lock()
				rep.Add(res)
				mu.Unlock()
			}(g, c)
		}
	}
	wg.Wait()
	rep.Finish()
	file := "report.json"
	if len(selected) == 1 {
		file = selected[0].Service + "-" + selected[0].Name + ".json"
	}
	if err := writeReport(cfg, rep, file); err != nil {
		return rep, err
	}
	publishReport(rep, cfg, file)
	return rep, nil
}

func publishReport(rep *report.Report, cfg harness.Config, file string) {
	published.mu.Lock()
	published.rep = rep
	published.cfg = cfg
	published.file = file
	published.mu.Unlock()
}

func noteCleanup(err error) error {
	if err == nil {
		return nil
	}
	published.mu.Lock()
	defer published.mu.Unlock()
	if published.rep != nil {
		published.rep.Add(report.Result{
			ID: "HARNESS-CLEANUP", Service: "harness", Operation: "Cleanup", Kind: "harness", Suite: "go-sdk",
			Status: "error", DocURL: cases.DocEC2("DeleteVpc"), Reason: err.Error(),
		})
		published.rep.Finish()
		if writeErr := writeReport(published.cfg, published.rep, published.file); writeErr != nil {
			return fmt.Errorf("write report: %w", writeErr)
		}
	}
	return err
}

var published struct {
	mu   sync.Mutex
	rep  *report.Report
	cfg  harness.Config
	file string
}

func runOne(ctx context.Context, rt *harness.Runtime, g bind.Group, c cases.Case) (res report.Result) {
	start := time.Now()
	res = report.Result{
		ID: c.ID, Service: c.Service, Operation: c.Op, Kind: c.Kind, Suite: c.Suite,
		DocURL: c.DocURL, Status: "error",
		Expected: map[string]any{"http": c.Expect.HTTP, "code": c.Expect.Code},
	}
	if c.Skip != "" {
		res.Status = "skip"
		res.Reason = c.Skip
		res.DurationMS = int(time.Since(start).Milliseconds())
		return res
	}
	caseRT := *rt
	caseRT.Clean = &harness.Cleaner{}
	defer func() {
		if rec := recover(); rec != nil {
			res.Status = "error"
			res.Reason = fmt.Sprint(rec)
		}
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), harness.OpTimeout)
		defer cancel()
		if err := caseRT.Clean.Run(cleanCtx); err != nil {
			res.Status = "error"
			if res.Reason == "" {
				res.Reason = err.Error()
			} else {
				res.Reason = res.Reason + "; " + err.Error()
			}
		}
		res.DurationMS = int(time.Since(start).Milliseconds())
	}()
	got, err := g.Run(ctx, &caseRT, c)
	res.Actual = map[string]any{"http": got.HTTP, "code": got.Code, "message": got.Message}
	if err != nil {
		res.Status = "error"
		res.Reason = err.Error()
		return res
	}
	status, reason := expect.Judge(c.Expect, got)
	res.Status = status
	res.Reason = reason
	if got.Raw != "" {
		ref := filepath.Join(rt.Cfg.OutDir, rt.Cfg.RunID, "raw", c.ID+".txt")
		if mkdirErr := os.MkdirAll(filepath.Dir(ref), 0o755); mkdirErr != nil {
			res.Status = "error"
			res.Reason = fmt.Errorf("raw artifact: %w", mkdirErr).Error()
			return res
		}
		if writeErr := os.WriteFile(ref, []byte(got.Raw), 0o644); writeErr != nil {
			res.Status = "error"
			res.Reason = fmt.Errorf("raw artifact: %w", writeErr).Error()
			return res
		}
		res.RawRef = ref
	}
	return res
}

func writeReport(cfg harness.Config, rep *report.Report, name string) error {
	dir := filepath.Join(cfg.OutDir, cfg.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return wrapErr("writeReport", err)
	}
	b, err := rep.JSON()
	if err != nil {
		return wrapErr("writeReport", err)
	}
	if schema := findSchema(); schema != "" {
		if err := report.Validate(schema, b); err != nil {
			return wrapErr("writeReport", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		return wrapErr("writeReport", err)
	}
	return wrapErr("writeReport", os.WriteFile(filepath.Join(dir, stringsTrim(name, ".json")+".md"),
		[]byte(report.Markdown(rep)), 0o644))
}

func findSchema() string {
	if p := os.Getenv("VWS_CONF_SCHEMA"); p != "" {
		return p
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "schema", "report.schema.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

func stringsTrim(s, suf string) string {
	if len(s) > len(suf) && s[len(s)-len(suf):] == suf {
		return s[:len(s)-len(suf)]
	}
	return s
}

func tools() report.Tools {
	sdk := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/aws/aws-sdk-go-v2" {
				sdk = d.Version
			}
		}
	}
	cli := "not-installed"
	return report.Tools{Go: goVersion(), SDK: sdk, CLI: cli}
}

func goVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.GoVersion
}
