package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

func TestCreateBucketRequiresVerifiedAbsence(t *testing.T) {
	var creates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				panic(err)
			}
		}
		if r.Method == http.MethodPut && !strings.Contains(strings.Trim(r.URL.Path, "/"), "/") {
			creates.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "head failed", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	rt := runtimeAt(t, srv.URL, t.TempDir(), "conf-20261015T120000Z-abc123")
	_, err := rt.S3CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String("conf-bucket")})
	if creates.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("creates %d err %v", creates.Load(), err)
	}
}

func TestInvalidActionDoesNotAuthorizeDelete(t *testing.T) {
	var deletes atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "DeleteInternetGateway" {
			deletes.Add(1)
			return 200, ec2OK("DeleteInternetGatewayResponse", "<return>true</return>")
		}
		if action == "DescribeInternetGateways" {
			return 400, ec2Err("InvalidAction", "DescribeInternetGateways is not served")
		}
		return ec2Describe(action, false)
	})
	dir := t.TempDir()
	writeLedger(t, dir, sweep.Resource{
		Kind: "internet-gateway", ID: "igw-1", LedgerRunID: "conf-20261015T120000Z-abc123",
		Target: "local-fake", Account: testAccount,
	})
	rt := ec2Runtime(t, srv, dir)
	err := rt.EC2DeleteInternetGateway(context.Background(), "igw-1")
	if deletes.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("deletes %d err %v", deletes.Load(), err)
	}
}

func TestNotFoundAfterForeignOwnerPreserves(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	const foreign = "conf-20261015T130000Z-def456"
	var deletes atomic.Int32
	var page atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "DeleteVolume" || action == "DetachVolume" {
			if action == "DeleteVolume" {
				deletes.Add(1)
			}
			return 200, ec2OK(action+"Response", "<return>true</return>")
		}
		if action == "DescribeVolumes" {
			if page.Add(1) == 1 {
				item := volumeItem("vol-1", foreign)
				return 200, ec2OK("DescribeVolumesResponse",
					"<volumeSet>"+item+"</volumeSet><nextToken>page2</nextToken>")
			}
			return 400, ec2Err("InvalidVolume.NotFound", "not found")
		}
		return ec2Describe(action, false)
	})
	dir := t.TempDir()
	writeLedger(t, dir, sweep.Resource{
		Kind: "volume", ID: "vol-1", LedgerRunID: run, Target: "local-fake", Account: testAccount,
	})
	rt := ec2Runtime(t, srv, dir)
	err := rt.EC2DeleteVolume(context.Background(), "vol-1")
	if page.Load() < 2 {
		t.Fatalf("second page was not observed, deletes %d err %v", deletes.Load(), err)
	}
	if deletes.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("deletes %d err %v", deletes.Load(), err)
	}
}

func TestTerminatePreservesWhenPeerDiscoveryFails(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var terms atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "TerminateInstances" {
			terms.Add(1)
			return 200, ec2OK("TerminateInstancesResponse", "<instancesSet/>")
		}
		if action == "DescribeInstances" {
			return 200, ec2OK("DescribeInstancesResponse", "<reservationSet>"+instOwned("i-1", run)+"</reservationSet>")
		}
		if action == "DescribeVolumes" {
			return 400, ec2Err("InvalidAction", "DescribeVolumes is not served")
		}
		return ec2Describe(action, false)
	})
	rt := ec2Runtime(t, srv, t.TempDir())
	err := rt.EC2Terminate(context.Background(), []string{"i-1"})
	if terms.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("terminates %d err %v", terms.Load(), err)
	}
}

func TestDisassociateMatchesObservedAssociation(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vals := requestValues(r)
		action := vals.Get("Action")
		if action == "DisassociateAddress" {
			sent = vals.Get("AssociationId")
			writeEC2(w, 200, ec2OK("DisassociateAddressResponse", "<return>true</return>"))
			return
		}
		if action == "DescribeInstances" {
			writeEC2(w, 200, ec2OK("DescribeInstancesResponse",
				"<reservationSet>"+instOwned("i-1", run)+"</reservationSet>"))
			return
		}
		if action == "DescribeAddresses" {
			item := addrItem("eipalloc-1", run, "eipassoc-real", "i-1")
			writeEC2(w, 200, ec2OK("DescribeAddressesResponse", "<addressesSet>"+item+"</addressesSet>"))
			return
		}
		status, body := ec2Describe(action, false)
		writeEC2(w, status, body)
	}))
	t.Cleanup(srv.Close)
	rt := ec2Runtime(t, srv.URL, t.TempDir())
	err := rt.EC2DisassociateAddress(context.Background(), "eipalloc-1", "eipassoc-other")
	if sent != "" || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("sent %q err %v", sent, err)
	}
}

func requestValues(r *http.Request) url.Values {
	if a := r.URL.Query().Get("Action"); a != "" {
		return r.URL.Query()
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	vals, err := url.ParseQuery(string(b))
	if err != nil {
		panic(err)
	}
	return vals
}

func TestFreshnessClearedOnDelete(t *testing.T) {
	const foreign = "conf-20261015T130000Z-def456"
	var deletes atomic.Int32
	var created atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		_ = body
		w.Header().Set("Content-Type", "application/xml")
		name := strings.Trim(r.URL.Path, "/")
		if r.Method == http.MethodPut && name == "conf-bucket" {
			created.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodDelete && name == "conf-bucket" {
			deletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Query().Has("tagging") {
			if _, err := io.WriteString(w, foreignTaggingXML(foreign)); err != nil {
				panic(err)
			}
			return
		}
		if r.Method == http.MethodHead || r.Method == http.MethodGet {
			if !created.Load() || deletes.Load() == 0 {
				w.WriteHeader(http.StatusNotFound)
				if _, err := io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>missing</Message></Error>`); err != nil {
					panic(err)
				}
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	rt := runtimeAt(t, srv.URL, t.TempDir(), "conf-20261015T120000Z-abc123")
	if _, err := rt.S3CreateBucket(context.Background(), &s3.CreateBucketInput{
		Bucket: aws.String("conf-bucket"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.S3DeleteBucket(context.Background(), "conf-bucket"); err != nil {
		t.Fatal(err)
	}
	err := rt.S3DeleteBucket(context.Background(), "conf-bucket")
	if deletes.Load() != 1 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("deletes %d err %v", deletes.Load(), err)
	}
}

func TestTerminateReconcilesBlockDeviceMappings(t *testing.T) {
	const run = "conf-20261015T120000Z-abc123"
	var terms atomic.Int32
	srv := httptestServer(t, func(action string) (int, string) {
		if action == "TerminateInstances" {
			terms.Add(1)
			return 200, ec2OK("TerminateInstancesResponse", "<instancesSet/>")
		}
		if action == "DescribeInstances" {
			item := strings.Replace(instOwned("i-1", run), "</instanceState>",
				"</instanceState><blockDeviceMapping><item><deviceName>/dev/xvda</deviceName>"+
					"<ebs><volumeId>vol-missing</volumeId><status>attached</status>"+
					"<deleteOnTermination>true</deleteOnTermination></ebs></item></blockDeviceMapping>", 1)
			return 200, ec2OK("DescribeInstancesResponse", "<reservationSet>"+item+"</reservationSet>")
		}
		if action == "DescribeVolumes" {
			return 200, ec2OK("DescribeVolumesResponse", "<volumeSet/>")
		}
		return ec2Describe(action, false)
	})
	rt := ec2Runtime(t, srv, t.TempDir())
	err := rt.EC2Terminate(context.Background(), []string{"i-1"})
	if terms.Load() != 0 || err == nil || !strings.Contains(err.Error(), "preserve") {
		t.Fatalf("terminates %d err %v", terms.Load(), err)
	}
}

func instOwned(id, run string) string {
	return "<item><reservationId>r-1</reservationId><ownerId>123456789012</ownerId><instancesSet><item>" +
		"<instanceId>" + id + "</instanceId><instanceState><code>16</code><name>running</name></instanceState>" +
		tagXML(run) + "</item></instancesSet></item>"
}

func writeLedger(t *testing.T, dir string, rows ...sweep.Resource) {
	t.Helper()
	var buf []byte
	for _, row := range rows {
		line, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func foreignTaggingXML(run string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Tagging><TagSet><Tag><Key>vws:conformance-run</Key><Value>` + run + `</Value></Tag></TagSet></Tagging>`
}
