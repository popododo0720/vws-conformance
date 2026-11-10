package harness

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/popododo0720/vws-conformance/internal/ec2fake"
)

func instanceInput() *ec2.RunInstancesInput {
	return &ec2.RunInstancesInput{
		ImageId: aws.String("ami-0123456789abcdef0"), MinCount: aws.Int32(1), MaxCount: aws.Int32(1),
	}
}

func TestInstanceGateOwnershipAndCleanup(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(fmt.Sprint(foreign), func(t *testing.T) {
			var deletes atomic.Int32
			var state atomic.Value
			state.Store("running")
			owner := testRun
			if foreign {
				owner = foreignRun
			}
			_, rt := newGateBehind(t, func(_ *ec2fake.Server, w http.ResponseWriter, r *http.Request) bool {
				action := r.Form.Get("Action")
				var body string
				switch action {
				case "RunInstances":
					if r.Form.Get("TagSpecification.1.Tag.1.Key") != RunTagKey ||
						r.Form.Get("TagSpecification.1.Tag.1.Value") != testRun {
						t.Error("missing run tag")
					}
					body = "<instancesSet><item><instanceId>i-0123456789abcdef0</instanceId></item></instancesSet>"
				case "DescribeInstances":
					if r.Form.Get("InstanceId.1") != "i-0123456789abcdef0" {
						t.Error("observer did not use exact ID")
					}
					body = fmt.Sprintf("<reservationSet><item><instancesSet><item><instanceId>i-0123456789abcdef0</"+
						"instanceId><instanceState><name>%s</name></instanceState><tagSet><item><key"+
						">%s</key><value>%s</value></item></tagSet></item></instancesSet></item></re"+
						"servationSet>", state.Load(), RunTagKey, owner)
				case "TerminateInstances":
					deletes.Add(1)
					state.Store("terminated")
					body = "<instancesSet><item><instanceId>i-0123456789abcdef0</instanceId><currentSta" +
						"te><name>terminated</name></currentState></item></instancesSet>"
				default:
					return false
				}
				if _, err := fmt.Fprintf(w, "<%sResponse>%s</%sResponse>", action, body, action); err != nil {
					t.Error(err)
				}
				return true
			})
			view := begin(t, rt, "EC2-RUN-001")
			in := instanceInput()
			out, err := view.EC2RunInstances(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if len(in.TagSpecifications) != 0 {
				t.Fatal("caller input changed")
			}
			if foreign {
				_, err = view.EC2TerminateInstances(context.Background(), aws.ToString(out.Instances[0].InstanceId))
				wantPreserve(t, err, "")
				if err := view.EndCase(context.Background()); err == nil {
					t.Fatal("foreign cleanup succeeded")
				}
				if deletes.Load() != 0 {
					t.Fatal("foreign instance terminated")
				}
				return
			}
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if deletes.Load() != 1 {
				t.Fatalf("terminations = %d", deletes.Load())
			}
		})
	}
}

func TestInstanceGateRejectsUnsafeInput(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EC2-RUN-001")
	if _, err := view.EC2RunInstances(context.Background(), nil); err == nil {
		t.Fatal("nil input accepted")
	}
	in := instanceInput()
	in.MaxCount = aws.Int32(2)
	if _, err := view.EC2RunInstances(context.Background(), in); err == nil {
		t.Fatal("multiple instances accepted")
	}
	in = instanceInput()
	in.TagSpecifications = []types.TagSpecification{{
		ResourceType: types.ResourceTypeInstance,
		Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
	}}
	if _, err := view.EC2RunInstances(context.Background(), in); err == nil {
		t.Fatal("forged ownership accepted")
	}
	if fake.Calls("RunInstances") != 0 {
		t.Fatal("unsafe create reached SDK")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := view.EC2TerminateInstances(context.Background(), "i-0123456789abcdef0"); err == nil {
		t.Fatal("closed scope accepted termination")
	}
}

func TestInstanceGateTreatsTerminatedAsGone(t *testing.T) {
	_, rt := newGateBehind(t, func(_ *ec2fake.Server, w http.ResponseWriter, r *http.Request) bool {
		if r.Form.Get("Action") != "DescribeInstances" {
			return false
		}
		_, err := fmt.Fprint(w, "<DescribeInstancesResponse><reservationSet><item><instancesSet><item><insta"+
			"nceId>i-0123456789abcdef0</instanceId><instanceState><name>terminated</name"+
			"></instanceState></item></instancesSet></item></reservationSet></DescribeIn"+
			"stancesResponse>")
		if err != nil {
			t.Error(err)
		}
		return true
	})
	obs := rt.observeRef(context.Background(), kinds[KindInstance], Ref{Kind: KindInstance, ID: "i-0123456789abcdef0"})
	if obs.Status != StatusAbsent {
		t.Fatalf("terminated observation = %+v", obs)
	}
	rec := Record{Ref: obs.Ref, State: StateDeleted}
	d := Decide(IntentCleanup, true, rec, obs)
	if d.Verdict != VerdictAlreadyGone {
		t.Fatalf("terminated instance after our terminate decided %+v from %+v", d, obs)
	}
	rec.State = StateCreated
	if d := Decide(IntentCleanup, true, rec, obs); d.Verdict != VerdictProceed {
		t.Fatalf("terminated instance after create decided %+v from %+v", d, obs)
	}
}
