package expect

import (
	"strings"
	"testing"

	"github.com/popododo0720/vws-conformance/internal/cases"
)

const running = `<instanceState><code>16</code><name>running</name></instanceState>`

func statusBody(inner string) string {
	return `<DescribeInstanceStatusResponse>` + inner + `</DescribeInstanceStatusResponse>`
}

func statusItems(items string) string {
	return statusBody(`<instanceStatusSet>` + items + `</instanceStatusSet>`)
}

func TestEC2List(t *testing.T) {
	const op, list = "DescribeInstanceStatus", "instanceStatusSet"
	req := []string{"instanceId", "instanceState/name"}
	tests := []struct {
		name, body, wantErr string
	}{
		{"empty body", "", "response body is empty"},
		{"blank body", "  \n", "response body is empty"},
		{"not xml", "{}", "not XML"},
		{"wrong root", `<DescribeTagsResponse><instanceStatusSet/></DescribeTagsResponse>`, "root element"},
		{"missing list", statusBody(`<requestId>r</requestId>`), `list element "instanceStatusSet" is missing`},
		{"item missing field", statusItems(`<item>` + running + `</item>`), "item 0 has no instanceId"},
		{"item empty nested field", statusItems(`<item><instanceId>i-1</instanceId>` +
			`<instanceState><name> </name></instanceState></item>`), "item 0 has no instanceState/name"},
		{"non-item child", statusItems(`<member/>`), `want "item"`},
		{"empty list", `<DescribeInstanceStatusResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
			`<requestId>r</requestId><instanceStatusSet/></DescribeInstanceStatusResponse>`, ""},
		{"full item", statusItems(`<item><instanceId>i-1</instanceId>` + running + `</item>`), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EC2List([]byte(tt.body), op, list, req...)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("got error %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("got error %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// A 200 response with an empty body must fail the case through Judge.
func TestShapeFieldFailsJudgeOnEmptyBody(t *testing.T) {
	exp := cases.Expect{HTTP: 200, Patterns: map[string]string{"shape": ShapeOKPattern}}
	got := cases.Outcome{HTTP: 200, Fields: map[string]string{}}
	got.Fields["shape"] = ShapeField(got, "DescribeRegions", "regionInfo", "regionName")
	if status, reason := Judge(exp, got); status != "fail" || !strings.Contains(reason, "empty") {
		t.Fatalf("Judge = %s %q, want fail on empty body", status, reason)
	}
	got.Body = []byte(`<DescribeRegionsResponse><regionInfo><item><regionName>r1</regionName></item>` +
		`</regionInfo></DescribeRegionsResponse>`)
	got.Fields["shape"] = ShapeField(got, "DescribeRegions", "regionInfo", "regionName")
	if status, reason := Judge(exp, got); status != "pass" {
		t.Fatalf("Judge = %s %q, want pass", status, reason)
	}
}

// An error outcome leaves the shape field empty so Judge reports the HTTP status.
func TestShapeFieldSkipsErrors(t *testing.T) {
	got := cases.Outcome{HTTP: 400, Code: "InvalidParameterValue"}
	if v := ShapeField(got, "DescribeRegions", "regionInfo"); v != "" {
		t.Fatalf("ShapeField on error = %q, want empty", v)
	}
}

func TestQueryNoResult(t *testing.T) {
	const ok = `<UntagPolicyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/">` +
		`<ResponseMetadata><RequestId>r-1</RequestId></ResponseMetadata></UntagPolicyResponse>`
	tests := []struct {
		name, body, wantErr string
	}{
		{"empty body", "", "response body is empty"},
		{"not xml", "{}", "not XML"},
		{"wrong root", `<TagPolicyResponse><ResponseMetadata><RequestId>r</RequestId></ResponseMetadata>` +
			`</TagPolicyResponse>`, "root element"},
		{"result element", `<UntagPolicyResponse><UntagPolicyResult/><ResponseMetadata><RequestId>r</RequestId>` +
			`</ResponseMetadata></UntagPolicyResponse>`, `unexpected element "UntagPolicyResult"`},
		{"no metadata", `<UntagPolicyResponse/>`, "no RequestId"},
		{"blank request id", `<UntagPolicyResponse><ResponseMetadata><RequestId> </RequestId>` +
			`</ResponseMetadata></UntagPolicyResponse>`, "no RequestId"},
		{"documented shape", ok, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := QueryNoResult([]byte(tt.body), "UntagPolicy")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
	if got := QueryNoResultField(cases.Outcome{HTTP: 404, Code: "NoSuchEntity"}, "UntagPolicy"); got != "" {
		t.Fatalf("error outcome shape = %q, want empty", got)
	}
	if got := QueryNoResultField(cases.Outcome{HTTP: 200, Body: []byte(ok)}, "UntagPolicy"); got != ShapeOK {
		t.Fatalf("shape = %q, want %q", got, ShapeOK)
	}
}
