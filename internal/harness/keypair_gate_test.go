package harness

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/popododo0720/vws-conformance/internal/ec2fake"
)

func TestKeyPairGateCreatesImportsAndCleansByID(t *testing.T) {
	for _, imported := range []bool{false, true} {
		t.Run(fmt.Sprint(imported), func(t *testing.T) {
			var deletes atomic.Int32
			var gone atomic.Bool
			_, rt := newGateBehind(t, func(_ *ec2fake.Server, w http.ResponseWriter, r *http.Request) bool {
				action := r.Form.Get("Action")
				var body string
				switch action {
				case "CreateKeyPair", "ImportKeyPair":
					if r.Form.Get("TagSpecification.1.ResourceType") != "key-pair" ||
						r.Form.Get("TagSpecification.1.Tag.1.Value") != testRun {
						t.Error("missing key-pair ownership")
					}
					body = "<keyPairId>key-0123456789abcdef0</keyPairId><keyName>test-key</keyName><key" +
						"Material>private-test-key</keyMaterial>"
				case "DescribeKeyPairs":
					if r.Form.Get("KeyPairId.1") != "key-0123456789abcdef0" || r.Form.Get("KeyName.1") != "" {
						t.Error("observer must use exact key-pair ID")
					}
					if gone.Load() {
						writeAPIError(t, w, http.StatusBadRequest, "InvalidKeyPair.NotFound")
						return true
					}
					body = fmt.Sprintf("<keySet><item><keyPairId>key-0123456789abcdef0</keyPairId><tagSet><item><ke"+
						"y>%s</key><value>%s</value></item></tagSet></item></keySet>", RunTagKey, testRun)
				case "DeleteKeyPair":
					if r.Form.Get("KeyPairId") != "key-0123456789abcdef0" || r.Form.Get("KeyName") != "" {
						t.Error("delete must use ID, not reusable name")
					}
					deletes.Add(1)
					gone.Store(true)
					body = "<return>true</return>"
				default:
					return false
				}
				if _, err := fmt.Fprintf(w, "<%sResponse>%s</%sResponse>", action, body, action); err != nil {
					t.Error(err)
				}
				return true
			})
			view := begin(t, rt, "EC2-KP-001")
			outcome, err := view.Call(context.Background(), func(ctx context.Context) error {
				if imported {
					_, err := view.EC2ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
						KeyName: aws.String(view.Name("key")), PublicKeyMaterial: []byte("test-public-key"),
					})
					return err
				}
				_, err := view.EC2CreateKeyPair(ctx, &ec2.CreateKeyPairInput{KeyName: aws.String(view.Name("key"))})
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(outcome.Raw, "private-test-key") {
				t.Fatal("private key leaked in capture")
			}
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
			if deletes.Load() != 1 {
				t.Fatalf("deletes = %d", deletes.Load())
			}
		})
	}
}

func TestKeyPairGatePreservesForeignAndMissingOwnership(t *testing.T) {
	for _, owner := range []string{foreignRun, ""} {
		t.Run(owner, func(t *testing.T) {
			var deletes atomic.Int32
			_, rt := newGateBehind(t, func(_ *ec2fake.Server, w http.ResponseWriter, r *http.Request) bool {
				switch r.Form.Get("Action") {
				case "DescribeKeyPairs":
					_, err := fmt.Fprintf(w, "<DescribeKeyPairsResponse><keySet><item><keyPairId>key-foreign</keyPairId><"+
						"tagSet><item><key>%s</key><value>%s</value></item></tagSet></item></keySet>"+
						"</DescribeKeyPairsResponse>", RunTagKey, owner)
					if err != nil {
						t.Error(err)
					}
					return true
				case "DeleteKeyPair":
					deletes.Add(1)
				}
				return false
			})
			view := begin(t, rt, "EC2-KP-003")
			_, err := view.EC2DeleteKeyPair(context.Background(), "key-foreign")
			wantPreserve(t, err, "")
			if deletes.Load() != 0 {
				t.Fatal("unowned key deleted")
			}
			if err := view.EndCase(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKeyPairGateRefusesForgedTagsAndClosedScopes(t *testing.T) {
	fake, rt := newGate(t)
	view := begin(t, rt, "EC2-KP-001")
	tags := []types.TagSpecification{{
		ResourceType: types.ResourceTypeKeyPair,
		Tags:         []types.Tag{{Key: aws.String(RunTagKey), Value: aws.String(foreignRun)}},
	}}
	createInput := &ec2.CreateKeyPairInput{TagSpecifications: tags}
	if _, err := view.EC2CreateKeyPair(context.Background(), createInput); err == nil {
		t.Fatal("forged create accepted")
	}
	importInput := &ec2.ImportKeyPairInput{TagSpecifications: tags}
	if _, err := view.EC2ImportKeyPair(context.Background(), importInput); err == nil {
		t.Fatal("forged import accepted")
	}
	if _, err := view.EC2CreateKeyPair(context.Background(), nil); err == nil {
		t.Fatal("nil create accepted")
	}
	if _, err := view.EC2ImportKeyPair(context.Background(), nil); err == nil {
		t.Fatal("nil import accepted")
	}
	if fake.Calls("CreateKeyPair") != 0 || fake.Calls("ImportKeyPair") != 0 {
		t.Fatal("unsafe create sent")
	}
	if err := view.EndCase(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := view.EC2DeleteKeyPair(context.Background(), "key-any"); err == nil {
		t.Fatal("closed scope accepted mutation")
	}
}
