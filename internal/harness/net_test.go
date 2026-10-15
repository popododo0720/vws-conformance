package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func TestEnsureNetSharesOneFailure(t *testing.T) {
	var creates atomic.Int32
	var inflight atomic.Int32
	var maxIn atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action != "CreateVpc" {
			writeEC2(w, 400, ec2Err("InvalidAction", action))
			return
		}
		n := inflight.Add(1)
		for {
			old := maxIn.Load()
			if n <= old || maxIn.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(150 * time.Millisecond)
		inflight.Add(-1)
		creates.Add(1)
		writeEC2(w, 500, ec2Err("ServerInternal", "failed"))
	}))
	defer srv.Close()
	rt := netRuntime(t, srv.URL)
	errA, errB := callTogether(rt)
	if errA == nil || errB == nil {
		t.Fatalf("errors %v %v", errA, errB)
	}
	if rt.net.vpc != "" || rt.net.subnet != "" {
		t.Fatalf("published %s %s", rt.net.vpc, rt.net.subnet)
	}
	if creates.Load() != 1 || maxIn.Load() != 1 {
		t.Fatalf("creates %d inflight %d", creates.Load(), maxIn.Load())
	}
}

func TestEnsureNetSharesOneNetwork(t *testing.T) {
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := ec2Action(r)
		if action == "CreateVpc" {
			creates.Add(1)
			time.Sleep(150 * time.Millisecond)
		}
		status, body := netReply(action)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	rt := netRuntime(t, srv.URL)
	type got struct {
		vpc, sub string
		err      error
	}
	var a, b got
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		a.vpc, a.sub, a.err = rt.EnsureNet(context.Background())
	}()
	go func() {
		defer wg.Done()
		<-start
		b.vpc, b.sub, b.err = rt.EnsureNet(context.Background())
	}()
	close(start)
	wg.Wait()
	if a.err != nil || b.err != nil {
		t.Fatalf("%v %v", a.err, b.err)
	}
	if a.vpc == "" || a.vpc != b.vpc || a.sub == "" || a.sub != b.sub {
		t.Fatalf("%s/%s vs %s/%s", a.vpc, a.sub, b.vpc, b.sub)
	}
	if creates.Load() != 1 {
		t.Fatalf("creates %d", creates.Load())
	}
}

func callTogether(rt *Runtime) (error, error) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	var errA, errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, _, errA = rt.EnsureNet(context.Background())
	}()
	go func() {
		defer wg.Done()
		<-start
		_, _, errB = rt.EnsureNet(context.Background())
	}()
	close(start)
	wg.Wait()
	return errA, errB
}

func netRuntime(t *testing.T, endpoint string) *Runtime {
	t.Helper()
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("vws-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "")),
		config.WithEC2IMDSClientEnableState(imds.ClientDisabled),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 1 })
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	return NewRuntime(Config{
		Region: "vws-1", OutDir: t.TempDir(),
		RunID: "conf-20261015T120000Z-abc123", Target: "local-fake",
	}, &Clients{EC2: client})
}

func netReply(action string) (int, string) {
	const ns = `xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"`
	switch action {
	case "CreateVpc":
		body := `<?xml version="1.0" encoding="UTF-8"?><CreateVpcResponse ` + ns + `>` +
			`<requestId>r</requestId><vpc><vpcId>vpc-1</vpcId><state>available</state>` +
			`<cidrBlock>10.0.0.0/16</cidrBlock><ownerId>123456789012</ownerId>` +
			`<isDefault>false</isDefault></vpc></CreateVpcResponse>`
		return 200, body
	case "CreateSubnet":
		body := `<?xml version="1.0" encoding="UTF-8"?><CreateSubnetResponse ` + ns + `>` +
			`<requestId>r</requestId><subnet><subnetId>subnet-1</subnetId><vpcId>vpc-1</vpcId>` +
			`<state>available</state><cidrBlock>10.0.1.0/24</cidrBlock>` +
			`<availabilityZone>vws-1a</availabilityZone></subnet></CreateSubnetResponse>`
		return 200, body
	default:
		return 400, ec2Err("InvalidAction", action)
	}
}
