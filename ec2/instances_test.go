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
	if _, err = describedInstance(out, "i-0000000000000001", 200); err == nil || !strings.Contains(err.Error(), "returned") {
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

func TestLaunchWaitsUntilRunning(t *testing.T) {
	var describes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(b))
		w.Header().Set("Content-Type", "text/xml")
		if vals.Get("Action") == "DescribeInstances" {
			n := describes.Add(1)
			state := "pending"
			if n >= 2 {
				state = "running"
			}
			_, _ = io.WriteString(w, describeInstanceXML("i-1", state))
			return
		}
		_, _ = io.WriteString(w, runInstanceXML("i-1"))
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
