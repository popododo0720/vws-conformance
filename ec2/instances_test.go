package ec2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func TestDescribedInstanceUsesResponse(t *testing.T) {
	launched := "i-0123456789abcdef0"
	out := &awsec2.DescribeInstancesOutput{Reservations: []types.Reservation{{
		Instances: []types.Instance{{InstanceId: aws.String(launched)}},
	}}}
	got, err := describedInstance(out, launched, 200)
	if err != nil || got != launched {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err = describedInstance(out, "i-0000000000000001", 200); err == nil || !strings.Contains(err.Error(),
		"returned") {
		t.Fatalf("mismatch err %v", err)
	}
	if _, err = describedInstance(&awsec2.DescribeInstancesOutput{}, launched, 200); err == nil {
		t.Fatal("empty success was accepted")
	}
	got, err = describedInstance(out, launched, 400)
	if err != nil || got != "" {
		t.Fatalf("http error became %q %v", got, err)
	}
}

func TestStartWaitsUntilStopped(t *testing.T) {
	var phase atomic.Int32
	var startedTooSoon atomic.Bool
	var started atomic.Bool
	var stopping atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		vals, queryErr := url.ParseQuery(string(b))
		if queryErr != nil {
			panic(queryErr)
		}
		w.Header().Set("Content-Type", "text/xml")
		switch vals.Get("Action") {
		case "CreateVpc":
			if _, writeErr := io.WriteString(w, ec2Item("CreateVpcResponse",
				"<vpc><vpcId>vpc-1</vpcId><state>available</state></vpc>")); writeErr != nil {
				panic(writeErr)
			}
		case "DescribeVpcs":
			if _, writeErr := io.WriteString(w, ec2Item("DescribeVpcsResponse", ownedVpcSet())); writeErr != nil {
				panic(writeErr)
			}
		case "CreateSubnet":
			inner := "<subnet><subnetId>subnet-1</subnetId><state>available</state><vpcId>vpc-1</vpcId></subnet>"
			if _, writeErr := io.WriteString(w, ec2Item("CreateSubnetResponse", inner)); writeErr != nil {
				panic(writeErr)
			}
		case "RunInstances":
			if _, writeErr := io.WriteString(w, runInstanceXML("i-1")); writeErr != nil {
				panic(writeErr)
			}
		case "StopInstances":
			phase.Store(1)
			if _, writeErr := io.WriteString(w, ec2Item("StopInstancesResponse", "<instancesSet/>")); writeErr != nil {
				panic(writeErr)
			}
		case "StartInstances":
			started.Store(true)
			if phase.Load() != 2 {
				startedTooSoon.Store(true)
			}
			if _, writeErr := io.WriteString(w, ec2Item("StartInstancesResponse", "<instancesSet/>")); writeErr != nil {
				panic(writeErr)
			}
		case "DescribeInstances":
			state := "running"
			if phase.Load() == 1 {
				if stopping.Add(1) >= 3 {
					phase.Store(2)
				}
				state = "stopping"
				if phase.Load() == 2 {
					state = "stopped"
				}
			} else if phase.Load() == 2 {
				state = "stopped"
			}
			if _, writeErr := io.WriteString(w, describeInstanceXML("i-1", state)); writeErr != nil {
				panic(writeErr)
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	cfg := harness.Config{
		AccessKey: "AKIDEXAMPLE", Secret: "secret", Region: "vws-1",
		EC2Endpoint: srv.URL, AllowedHosts: []string{"127.0.0.1"},
		AllowedAccounts: []string{"123456789012"},
		Target:          "local-fake", RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(),
	}
	cl, err := harness.Load(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := harness.NewRuntime(cfg, cl)
	_, err = runInstances(context.Background(), rt, cases.Case{ID: "EC2-START-001"})
	if err != nil {
		t.Fatal(err)
	}
	if !started.Load() || startedTooSoon.Load() {
		t.Fatalf("started %v too soon %v stopping describes %d", started.Load(), startedTooSoon.Load(), stopping.Load())
	}
}

func ownedVpcSet() string {
	return `<vpcSet><item><vpcId>vpc-1</vpcId><state>available</state><cidrBlock>10.0.0.0/16</cidrBlock>` +
		`<tagSet><item><key>vws:conformance-run</key>` +
		`<value>conf-20261015T120000Z-abc123</value></item></tagSet></item></vpcSet>`
}

func ec2Item(root, inner string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<` + root + ` xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId>` + inner + `</` + root + `>`
}

func TestLaunchWaitsUntilRunning(t *testing.T) {
	var describes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			panic(readErr)
		}
		vals, queryErr := url.ParseQuery(string(b))
		if queryErr != nil {
			panic(queryErr)
		}
		w.Header().Set("Content-Type", "text/xml")
		if vals.Get("Action") == "DescribeInstances" {
			n := describes.Add(1)
			state := "pending"
			if n >= 2 {
				state = "running"
			}
			if _, writeErr := io.WriteString(w, describeInstanceXML("i-1", state)); writeErr != nil {
				panic(writeErr)
			}
			return
		}
		if _, writeErr := io.WriteString(w, runInstanceXML("i-1")); writeErr != nil {
			panic(writeErr)
		}
	}))
	defer srv.Close()
	rt := harness.NewRuntime(harness.Config{
		RunID: "conf-20261015T120000Z-abc123", OutDir: t.TempDir(), Region: "vws-1",
	}, nil)
	id, err := launch(context.Background(), rt, testEC2(t, srv.URL), "subnet-1", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if id != "i-1" || describes.Load() < 2 {
		t.Fatalf("id %s describes %d", id, describes.Load())
	}
}

func runInstanceXML(id string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<RunInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId><reservationId>r-1</reservationId><ownerId>123456789012</ownerId>` +
		`<instancesSet><item><instanceId>` + id + `</instanceId>` +
		`<instanceState><code>0</code><name>pending</name></instanceState>` +
		`</item></instancesSet></RunInstancesResponse>`
}

func describeInstanceXML(id, state string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<DescribeInstancesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15/">` +
		`<requestId>r</requestId><reservationSet><item>` +
		`<reservationId>r-1</reservationId><ownerId>123456789012</ownerId>` +
		`<instancesSet><item><instanceId>` + id + `</instanceId>` +
		`<instanceState><code>16</code><name>` + state + `</name></instanceState>` +
		`</item></instancesSet></item></reservationSet></DescribeInstancesResponse>`
}
