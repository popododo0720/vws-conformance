package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Target struct {
	Name              string            `json:"name"`
	Region            string            `json:"region"`
	ConsoleAPIVersion string            `json:"console_api_version"`
	ServiceImages     map[string]string `json:"service_images,omitempty"`
}

type Tools struct {
	Go  string `json:"go"`
	SDK string `json:"aws_sdk_go_v2"`
	CLI string `json:"aws_cli"`
}

type Summary struct {
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
	Error int `json:"error"`
	Total int `json:"total"`
}

type Result struct {
	ID         string         `json:"id"`
	Service    string         `json:"service"`
	Operation  string         `json:"operation"`
	Kind       string         `json:"kind"`
	Suite      string         `json:"suite"`
	Status     string         `json:"status"`
	DocURL     string         `json:"doc_url"`
	Expected   map[string]any `json:"expected,omitempty"`
	Actual     map[string]any `json:"actual,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Issue      string         `json:"issue,omitempty"`
	RawRef     string         `json:"raw_ref,omitempty"`
	DurationMS int            `json:"duration_ms"`
}

type Report struct {
	SchemaVersion string   `json:"schema_version"`
	RunID         string   `json:"run_id"`
	StartedAt     string   `json:"started_at"`
	FinishedAt    string   `json:"finished_at"`
	Target        Target   `json:"target"`
	ToolVersions  Tools    `json:"tool_versions"`
	Summary       Summary  `json:"summary"`
	Results       []Result `json:"results"`
}

func New(runID string, target Target, tools Tools) *Report {
	return &Report{
		SchemaVersion: "1",
		RunID:         runID,
		StartedAt:     time.Now().UTC().Format(time.RFC3339),
		Target:        target,
		ToolVersions:  tools,
		Results:       []Result{},
	}
}

func (r *Report) Add(res Result) {
	r.Results = append(r.Results, res)
}

func Merge(parts []*Report) (*Report, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("no reports to merge")
	}
	out := *parts[0]
	out.Results = append([]Result(nil), parts[0].Results...)
	for _, p := range parts[1:] {
		if p.RunID != out.RunID {
			return nil, fmt.Errorf("run id %s does not match %s", p.RunID, out.RunID)
		}
		out.Results = append(out.Results, p.Results...)
	}
	out.Finish()
	return &out, nil
}

func (r *Report) Finish() {
	r.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	r.Summary = Summary{}
	for _, res := range r.Results {
		r.Summary.Total++
		switch res.Status {
		case "pass":
			r.Summary.Pass++
		case "fail":
			r.Summary.Fail++
		case "skip":
			r.Summary.Skip++
		case "error":
			r.Summary.Error++
		}
	}
}

func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

func Validate(schemaPath string, doc []byte) error {
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		return fmt.Errorf("report json: %w", err)
	}
	c := jsonschema.NewCompiler()
	f, err := os.Open(schemaPath)
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	defer f.Close()
	schDoc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		return fmt.Errorf("schema json: %w", err)
	}
	if err := c.AddResource("report.schema.json", schDoc); err != nil {
		return err
	}
	sch, err := c.Compile("report.schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := sch.Validate(v); err != nil {
		return fmt.Errorf("report: %w", err)
	}
	return nil
}

func Markdown(r *Report) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# Conformance %s\n\n", r.RunID)
	fmt.Fprintf(&b, "Target %s, region %s, console-api %s.\n\n", r.Target.Name, r.Target.Region, r.Target.ConsoleAPIVersion)
	fmt.Fprintf(&b, "Go %s, aws-sdk-go-v2 %s, aws cli %s.\n\n", r.ToolVersions.Go, r.ToolVersions.SDK, r.ToolVersions.CLI)
	fmt.Fprintf(&b, "Pass %d, fail %d, skip %d, error %d, total %d.\n\n", r.Summary.Pass, r.Summary.Fail, r.Summary.Skip, r.Summary.Error, r.Summary.Total)
	by := map[string][]Result{}
	for _, res := range r.Results {
		by[res.Service] = append(by[res.Service], res)
	}
	services := make([]string, 0, len(by))
	for s := range by {
		services = append(services, s)
	}
	sort.Strings(services)
	rank := map[string]int{"fail": 0, "error": 1, "skip": 2, "pass": 3}
	for _, svc := range services {
		rows := by[svc]
		sort.Slice(rows, func(i, j int) bool {
			if rank[rows[i].Status] != rank[rows[j].Status] {
				return rank[rows[i].Status] < rank[rows[j].Status]
			}
			return rows[i].ID < rows[j].ID
		})
		fmt.Fprintf(&b, "## %s\n\n", svc)
		b.WriteString("| ID | Operation | Kind | Suite | Status | Expected | Actual | Doc | Issue |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, row := range rows {
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				row.ID, row.Operation, row.Kind, row.Suite, row.Status,
				cell(row.Expected), cell(row.Actual), row.DocURL, row.Issue)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Gaps\n\n")
	b.WriteString("| ID | Operation | Status | Reason |\n| --- | --- | --- | --- |\n")
	for _, res := range r.Results {
		if res.Kind == "gap" {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", res.ID, res.Operation, res.Status, strings.ReplaceAll(res.Reason, "|", "/"))
		}
	}
	b.WriteString("\n")
	return b.String()
}

func cell(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	s := string(b)
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func IssueText(r *Report, res Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: [parity] %s %s: %s (%s)\n\n", res.Service, res.Operation, oneLine(res.Reason), res.ID)
	fmt.Fprintf(&b, "Case: %s (suite %s)\n", res.ID, res.Suite)
	fmt.Fprintf(&b, "AWS reference: %s\n", res.DocURL)
	fmt.Fprintf(&b, "Run: %s on %s, console-api %s, aws-sdk-go-v2 %s, aws-cli %s\n\n", r.RunID, r.Target.Name, r.Target.ConsoleAPIVersion, r.ToolVersions.SDK, r.ToolVersions.CLI)
	fmt.Fprintf(&b, "Expected (AWS): %s\n", cell(res.Expected))
	fmt.Fprintf(&b, "Actual (VWS): %s\n\n", cell(res.Actual))
	fmt.Fprintf(&b, "Redacted request/response: %s\n", res.RawRef)
	fmt.Fprintf(&b, "Reproduce: go test ./%s -args -service %s\n\n", res.Service, res.Service)
	fmt.Fprintf(&b, "Acceptance: case %s passes on vws-test; no other case regresses.\n", res.ID)
	b.WriteString("Part of #1\n")
	return b.String()
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "response differs from AWS"
	}
	return s
}
