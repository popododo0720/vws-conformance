package harness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	mutationRun = "conf-20261015T120000Z-abc123"
	foreignRun  = "conf-20261015T130000Z-def456"
	activeRun   = "conf-20261015T140000Z-aaa111"
	runPrefix   = "conf-20261015t120000z-abc123"
)

type mutationRow struct {
	name       string
	instances  string
	volumes    string
	addresses  string
	snaps      string
	keys       string
	groups     string
	subnets    string
	vpcs       string
	gateways   string
	bucket     string
	active     string
	direct     string
	directErr  bool
	directAddr string
	ledger     string
	ledgerKind string
	forbid     []string
	require    []string
	dependency []string
	wantErr    bool
}

func TestMutationStopsWhenOwnershipIsUnverified(t *testing.T) {
	for _, row := range mutationRows() {
		t.Run(row.name, func(t *testing.T) {
			runMutationRow(t, row)
		})
	}
}

func mutationRows() []mutationRow {
	return []mutationRow{
		{
			name: "volume attached to a foreign instance", wantErr: true,
			instances: instItem("i-peer", foreignRun, "running"),
			volumes:   volItem("vol-peer", mutationRun, "i-peer", "attached"),
			forbid:    []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "volume attached to an untagged instance", wantErr: true,
			instances: instItem("i-peer", "", "running"),
			volumes:   volItem("vol-peer", mutationRun, "i-peer", "attached"),
			forbid:    []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "volume attached to an active run", wantErr: true, active: activeRun,
			instances: instItem("i-peer", activeRun, "running"),
			volumes:   volItem("vol-peer", mutationRun, "i-peer", "attached"),
			forbid:    []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name:      "volume attached to this run",
			instances: instItem("i-peer", mutationRun, "running"),
			volumes:   volItem("vol-peer", mutationRun, "i-peer", "attached"),
			require:   []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name:    "unattached volume",
			volumes: volItem("vol-peer", mutationRun, "", ""),
			require: []string{"DeleteVolume"},
		},
		{
			name:      "detached volume keeps a foreign instance",
			instances: instItem("i-peer", foreignRun, "running"),
			volumes:   volItem("vol-peer", mutationRun, "i-peer", "detached"),
			require:   []string{"DeleteVolume"},
		},
		{
			name: "address associated to a foreign instance", wantErr: true,
			instances: instItem("i-peer", foreignRun, "running"),
			addresses: addrItem("eipalloc-peer", mutationRun, "eipassoc-peer", "i-peer"),
			forbid:    []string{"DisassociateAddress", "ReleaseAddress"},
		},
		{
			name: "address association without an instance", wantErr: true,
			addresses: addrItem("eipalloc-peer", mutationRun, "eipassoc-peer", ""),
			forbid:    []string{"DisassociateAddress", "ReleaseAddress"},
		},
		{
			name:      "address associated to this run",
			instances: instItem("i-peer", mutationRun, "running"),
			addresses: addrItem("eipalloc-peer", mutationRun, "eipassoc-peer", "i-peer"),
			require:   []string{"DisassociateAddress", "ReleaseAddress"},
		},
		{
			name:      "unassociated address",
			addresses: addrItem("eipalloc-peer", mutationRun, "", ""),
			require:   []string{"ReleaseAddress"},
		},
		{
			name: "gateway attached to a foreign vpc", wantErr: true,
			vpcs:     vpcItem("vpc-peer", foreignRun),
			gateways: igwItem("igw-peer", mutationRun, "vpc-peer"),
			forbid:   []string{"DetachInternetGateway", "DeleteInternetGateway"},
		},
		{
			name: "gateway attached to an untagged vpc", wantErr: true,
			vpcs:     vpcItem("vpc-peer", ""),
			gateways: igwItem("igw-peer", mutationRun, "vpc-peer"),
			forbid:   []string{"DetachInternetGateway", "DeleteInternetGateway"},
		},
		{
			name:     "gateway attached to this run",
			vpcs:     vpcItem("vpc-peer", mutationRun),
			gateways: igwItem("igw-peer", mutationRun, "vpc-peer"),
			require:  []string{"DetachInternetGateway", "DeleteInternetGateway"},
		},
		{
			name:     "unattached gateway",
			gateways: igwItem("igw-peer", mutationRun, ""),
			require:  []string{"DeleteInternetGateway"},
		},
		{
			name: "instance attached to a foreign volume", wantErr: true,
			instances: instItem("i-peer", mutationRun, "running"),
			volumes:   volItem("vol-foreign", foreignRun, "i-peer", "attached"),
			forbid:    []string{"TerminateInstances", "DetachVolume", "DeleteVolume"},
		},
		{
			name: "preserved instance does not retry the subnet", wantErr: true,
			instances:  instItem("i-peer", mutationRun, "running"),
			volumes:    volItem("vol-foreign", foreignRun, "i-peer", "attached"),
			subnets:    subnetItem("subnet-peer", mutationRun),
			dependency: []string{"DeleteSubnet"},
			forbid:     []string{"TerminateInstances"},
		},
		{
			name:      "instance with no attachment",
			instances: instItem("i-peer", mutationRun, "running"),
			require:   []string{"TerminateInstances"},
		},
		{
			name: "root volume without provenance preserves the instance", wantErr: true,
			instances: instItem("i-peer", mutationRun, "running"),
			volumes:   volItemTerm("vol-root", "", "i-peer", "attached", "true"),
			forbid:    []string{"TerminateInstances", "DeleteVolume", "DetachVolume"},
		},
		{
			name:      "recorded launch volume allows termination",
			instances: instItem("i-peer", mutationRun, "running"),
			volumes:   volItemTerm("vol-root", "", "i-peer", "attached", "true"),
			ledger:    "vol-root",
			require:   []string{"TerminateInstances"},
		},
		{
			name: "volume attachment without an instance preserves it", wantErr: true,
			volumes: volOpen("vol-peer", mutationRun),
			forbid:  []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "gateway attachment without a vpc preserves it", wantErr: true,
			gateways: igwOpen("igw-peer", mutationRun),
			forbid:   []string{"DetachInternetGateway", "DeleteInternetGateway"},
		},
		{
			name:    "malformed explicit owner preserves the volume",
			volumes: volBadOwner("vol-peer", mutationRun),
			forbid:  []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "ledger volume is preserved when a direct describe shows a foreign instance", wantErr: true,
			instances: instItem("i-peer", foreignRun, "running"),
			direct:    volItem("vol-peer", mutationRun, "i-peer", "attached"),
			ledger:    "vol-peer",
			forbid:    []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "ledger volume is preserved when a direct describe fails", wantErr: true,
			directErr: true, ledger: "vol-peer",
			forbid: []string{"DetachVolume", "DeleteVolume"},
		},
		{
			name: "ledger address is preserved when a direct describe shows a foreign instance", wantErr: true,
			instances:  instItem("i-peer", foreignRun, "running"),
			directAddr: addrItem("eipalloc-peer", mutationRun, "eipassoc-peer", "i-peer"),
			ledger:     "eipalloc-peer", ledgerKind: "address",
			forbid: []string{"DisassociateAddress", "ReleaseAddress"},
		},
		{
			name: "untagged data volume blocks instance termination", wantErr: true,
			instances: instItem("i-peer", mutationRun, "running"),
			volumes:   volItemTerm("vol-data", "", "i-peer", "attached", "false"),
			forbid:    []string{"TerminateInstances", "DeleteVolume", "DetachVolume"},
		},
		{
			name:    "snapshot this run owns",
			snaps:   snapItem("snap-peer", mutationRun),
			require: []string{"DeleteSnapshot"},
		},
		{
			name:    "key pair this run owns",
			keys:    keyItem(runPrefix + "-key"),
			require: []string{"DeleteKeyPair"},
		},
		{
			name:    "security group this run owns",
			groups:  groupItem("sg-peer", runPrefix+"-sg"),
			require: []string{"DeleteSecurityGroup"},
		},
		{
			name:    "subnet this run owns",
			subnets: subnetItem("subnet-peer", mutationRun),
			require: []string{"DeleteSubnet"},
		},
		{
			name:    "vpc this run owns",
			vpcs:    vpcItem("vpc-peer", mutationRun),
			require: []string{"DeleteVpc"},
		},
		{
			name:    "bucket this run owns",
			bucket:  runPrefix + "-bkt",
			require: []string{"DeleteBucket"},
		},
	}
}

func runMutationRow(t *testing.T, row mutationRow) {
	t.Helper()
	if row.active != "" {
		dir := t.TempDir()
		t.Setenv("VWS_CONF_ACTIVITY_DIR", dir)
		if err := os.WriteFile(filepath.Join(dir, row.active), []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	seen := map[string]int{}
	removed := map[string]bool{}
	cleared := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("versions") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotImplemented)
			if _, err := w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
				`<Error><Code>NotImplemented</Code><Message>not served</Message></Error>`)); err != nil {
				panic(err)
			}
			return
		}
		if writeNoSuchTagSet(w, r) {
			return
		}
		if r.URL.Host == "" && (r.Method == http.MethodGet || r.Method == http.MethodDelete) &&
			!strings.Contains(r.URL.RawQuery, "Action") && ec2ActionQuery(r) == "" {
			serveBucketMutation(w, r, row.bucket, &mu, seen, &cleared)
			return
		}
		vals := ec2Form(r)
		action := vals.Get("Action")
		if action == "" {
			serveBucketMutation(w, r, row.bucket, &mu, seen, &cleared)
			return
		}
		if action == "DescribeVolumes" && vals.Get("VolumeId.1") != "" {
			if row.directErr {
				writeEC2(w, http.StatusInternalServerError, ec2Err("InternalError", "unread"))
				return
			}
			if row.direct != "" {
				writeEC2(w, http.StatusOK, ec2OK("DescribeVolumesResponse", "<volumeSet>"+row.direct+"</volumeSet>"))
				return
			}
		}
		if action == "DescribeAddresses" && vals.Get("AllocationId.1") != "" && row.directAddr != "" {
			writeEC2(w, http.StatusOK, ec2OK("DescribeAddressesResponse", "<addressesSet>"+row.directAddr+"</addressesSet>"))
			return
		}
		mu.Lock()
		if isMutation(action) {
			seen[action]++
		}
		if listed(row.dependency, action) {
			mu.Unlock()
			writeEC2(w, http.StatusBadRequest, ec2Err("DependencyViolation", "still in use"))
			return
		}
		gone := targetedGone(action, vals, removed)
		if isTerminal(action) {
			if id := mutatedID(action, vals); id != "" {
				removed[id] = true
			}
		}
		hidden := map[string]bool{}
		for id := range removed {
			hidden[id] = true
		}
		mu.Unlock()
		if gone {
			writeEC2(w, http.StatusBadRequest, ec2Err("Invalid"+goneKind(action)+".NotFound", "gone"))
			return
		}
		if isMutation(action) {
			writeEC2(w, http.StatusOK, ec2OK(action+"Response", "<return>true</return>"))
			return
		}
		status, body := mutationDescribe(action, row, hidden)
		writeEC2(w, status, body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	if row.ledger != "" {
		kind := row.ledgerKind
		if kind == "" {
			kind = "volume"
		}
		line := `{"Kind":"` + kind + `","ID":"` + row.ledger + `","LedgerRunID":"` + mutationRun +
			`","Target":"local-fake","Account":"` + testAccount + `"}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, "ledger.jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rt := runtimeAt(t, srv.URL, dir, mutationRun)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := rt.Sweep(ctx)
	mu.Lock()
	defer mu.Unlock()
	if row.wantErr {
		if err == nil || !strings.Contains(err.Error(), "preserve") {
			t.Fatalf("err %v actions %v", err, seen)
		}
		for action, n := range seen {
			if n > 1 {
				t.Fatalf("retried %s %d times, actions %v", action, n, seen)
			}
		}
	} else if err != nil {
		t.Fatalf("err %v actions %v", err, seen)
	}
	for _, action := range row.forbid {
		if seen[action] != 0 {
			t.Fatalf("called %s %d times, actions %v", action, seen[action], seen)
		}
	}
	for _, action := range row.require {
		if seen[action] == 0 {
			t.Fatalf("missing %s actions %v err %v", action, seen, err)
		}
	}
}

func ec2ActionQuery(r *http.Request) string {
	return r.URL.Query().Get("Action")
}

func serveBucketMutation(
	w http.ResponseWriter, r *http.Request, bucket string, mu *sync.Mutex, seen map[string]int, cleared *bool,
) {
	w.Header().Set("Content-Type", "application/xml")
	path := strings.Trim(r.URL.Path, "/")
	if r.Method == http.MethodDelete && path == bucket {
		mu.Lock()
		seen["DeleteBucket"]++
		*cleared = true
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet && path == bucket {
		if _, err := w.Write([]byte(emptyListXML(bucket))); err != nil {
			panic(err)
		}
		return
	}
	mu.Lock()
	name := bucket
	if *cleared {
		name = ""
	}
	mu.Unlock()
	names := map[string]bool{}
	if name != "" {
		names[name] = true
	}
	if _, err := w.Write([]byte(listBucketsXML(names))); err != nil {
		panic(err)
	}
}

func mutationDescribe(action string, row mutationRow, removed map[string]bool) (int, string) {
	row.instances = hideOwned(row.instances, "instanceId", removed)
	row.volumes = hideOwned(row.volumes, "volumeId", removed)
	row.addresses = hideOwned(row.addresses, "allocationId", removed)
	row.snaps = hideOwned(row.snaps, "snapshotId", removed)
	row.keys = hideOwned(row.keys, "keyName", removed)
	row.groups = hideOwned(row.groups, "groupId", removed)
	row.subnets = hideOwned(row.subnets, "subnetId", removed)
	row.vpcs = hideOwned(row.vpcs, "vpcId", removed)
	row.gateways = hideOwned(row.gateways, "internetGatewayId", removed)
	switch action {
	case "DescribeInstances":
		return 200, ec2OK("DescribeInstancesResponse", "<reservationSet>"+row.instances+"</reservationSet>")
	case "DescribeVolumes":
		return 200, ec2OK("DescribeVolumesResponse", "<volumeSet>"+row.volumes+"</volumeSet>")
	case "DescribeSnapshots":
		return 200, ec2OK("DescribeSnapshotsResponse", "<snapshotSet>"+row.snaps+"</snapshotSet>")
	case "DescribeAddresses":
		return 200, ec2OK("DescribeAddressesResponse", "<addressesSet>"+row.addresses+"</addressesSet>")
	case "DescribeKeyPairs":
		return 200, ec2OK("DescribeKeyPairsResponse", "<keySet>"+row.keys+"</keySet>")
	case "DescribeSecurityGroups":
		return 200, ec2OK("DescribeSecurityGroupsResponse", "<securityGroupInfo>"+row.groups+"</securityGroupInfo>")
	case "DescribeSubnets":
		return 200, ec2OK("DescribeSubnetsResponse", "<subnetSet>"+row.subnets+"</subnetSet>")
	case "DescribeVpcs":
		return 200, ec2OK("DescribeVpcsResponse", "<vpcSet>"+row.vpcs+"</vpcSet>")
	case "DescribeInternetGateways":
		return 200, ec2OK("DescribeInternetGatewaysResponse", "<internetGatewaySet>"+row.gateways+"</internetGatewaySet>")
	default:
		return 400, ec2Err("InvalidAction", action)
	}
}

func hideOwned(xml, idTag string, removed map[string]bool) string {
	for id := range removed {
		if strings.Contains(xml, "<"+idTag+">"+id+"</"+idTag+">") {
			return ""
		}
	}
	return xml
}

func mutatedID(action string, vals url.Values) string {
	switch action {
	case "TerminateInstances":
		return firstValue(vals, "InstanceId.1", "InstanceId")
	case "DeleteVolume":
		return firstValue(vals, "VolumeId", "VolumeId.1")
	case "ReleaseAddress":
		return firstValue(vals, "AllocationId", "AllocationId.1")
	case "DeleteInternetGateway":
		return firstValue(vals, "InternetGatewayId", "InternetGatewayId.1")
	case "DeleteSnapshot":
		return firstValue(vals, "SnapshotId", "SnapshotId.1")
	case "DeleteKeyPair":
		return firstValue(vals, "KeyName", "KeyName.1")
	case "DeleteSecurityGroup":
		return firstValue(vals, "GroupId", "GroupId.1")
	case "DeleteSubnet":
		return firstValue(vals, "SubnetId", "SubnetId.1")
	case "DeleteVpc":
		return firstValue(vals, "VpcId", "VpcId.1")
	default:
		return ""
	}
}

func firstValue(vals url.Values, keys ...string) string {
	for _, key := range keys {
		if v := vals.Get(key); v != "" {
			return v
		}
	}
	return ""
}

func goneKind(action string) string {
	switch action {
	case "DescribeVolumes":
		return "Volume"
	case "DescribeInstances":
		return "InstanceID"
	case "DescribeAddresses":
		return "AllocationID"
	case "DescribeInternetGateways":
		return "InternetGatewayID"
	case "DescribeSnapshots":
		return "Snapshot"
	case "DescribeKeyPairs":
		return "KeyPair"
	case "DescribeSecurityGroups":
		return "Group"
	case "DescribeSubnets":
		return "SubnetID"
	case "DescribeVpcs":
		return "VpcID"
	default:
		return "Resource"
	}
}

func targetedGone(action string, vals url.Values, removed map[string]bool) bool {
	var id string
	switch action {
	case "DescribeVolumes":
		id = vals.Get("VolumeId.1")
	case "DescribeInstances":
		id = vals.Get("InstanceId.1")
	case "DescribeAddresses":
		id = vals.Get("AllocationId.1")
	case "DescribeInternetGateways":
		id = vals.Get("InternetGatewayId.1")
	case "DescribeSnapshots":
		id = vals.Get("SnapshotId.1")
	case "DescribeKeyPairs":
		id = vals.Get("KeyName.1")
	case "DescribeSecurityGroups":
		id = vals.Get("GroupId.1")
	case "DescribeSubnets":
		id = vals.Get("SubnetId.1")
	case "DescribeVpcs":
		id = vals.Get("VpcId.1")
	}
	return id != "" && removed[id]
}

func listed(items []string, action string) bool {
	for _, item := range items {
		if item == action {
			return true
		}
	}
	return false
}

func isMutation(action string) bool {
	switch action {
	case "DetachVolume", "DeleteVolume", "DisassociateAddress", "ReleaseAddress",
		"DetachInternetGateway", "DeleteInternetGateway", "TerminateInstances",
		"DeleteSnapshot", "DeleteKeyPair", "DeleteSecurityGroup", "DeleteSubnet", "DeleteVpc":
		return true
	default:
		return false
	}
}

func isTerminal(action string) bool {
	switch action {
	case "DeleteVolume", "ReleaseAddress", "DeleteInternetGateway", "TerminateInstances",
		"DeleteSnapshot", "DeleteKeyPair", "DeleteSecurityGroup", "DeleteSubnet", "DeleteVpc":
		return true
	default:
		return false
	}
}

func volOpen(id, owner string) string {
	att := "<attachmentSet><item><volumeId>" + id + "</volumeId><status>attached</status></item></attachmentSet>"
	return "<item><volumeId>" + id + "</volumeId><size>1</size><availabilityZone>vws-1a</availabilityZone>" +
		"<status>in-use</status><volumeType>gp2</volumeType>" + att + tagXML(owner) + "</item>"
}

func volBadOwner(id, owner string) string {
	item := volItem(id, owner, "", "")
	return strings.Replace(item, "</tagSet>",
		"<item><key>vws:conformance-run</key><value>not-a-run</value></item></tagSet>", 1)
}

func igwOpen(id, owner string) string {
	att := "<attachmentSet><item><state>available</state></item></attachmentSet>"
	return "<item><internetGatewayId>" + id + "</internetGatewayId>" + att + tagXML(owner) + "</item>"
}

func volItemTerm(id, owner, inst, state, del string) string {
	item := volItem(id, owner, inst, state)
	if del == "" {
		return item
	}
	return strings.Replace(item, "</status></item></attachmentSet>",
		"</status><deleteOnTermination>"+del+"</deleteOnTermination></item></attachmentSet>", 1)
}

func volItem(id, owner, inst, state string) string {
	att := ""
	if inst != "" {
		att = "<attachmentSet><item><volumeId>" + id + "</volumeId><instanceId>" + inst +
			"</instanceId><device>/dev/sdf</device><status>" + state + "</status></item></attachmentSet>"
	}
	tags := ""
	if owner != "" {
		tags = tagXML(owner)
	}
	return "<item><volumeId>" + id + "</volumeId><size>1</size><availabilityZone>vws-1a</availabilityZone>" +
		"<status>in-use</status><volumeType>gp2</volumeType>" + att + tags + "</item>"
}

func instItem(id, owner, state string) string {
	tags := ""
	if owner != "" {
		tags = tagXML(owner)
	}
	return "<item><reservationId>r-1</reservationId><ownerId>123456789012</ownerId><instancesSet><item>" +
		"<instanceId>" + id + "</instanceId><instanceState><code>16</code><name>" + state +
		"</name></instanceState>" + tags + "</item></instancesSet></item>"
}

func addrItem(id, owner, assoc, inst string) string {
	extra := ""
	if assoc != "" {
		extra += "<associationId>" + assoc + "</associationId>"
	}
	if inst != "" {
		extra += "<instanceId>" + inst + "</instanceId>"
	}
	tags := ""
	if owner != "" {
		tags = tagXML(owner)
	}
	return "<item><allocationId>" + id + "</allocationId><domain>vpc</domain>" +
		"<publicIp>203.0.113.10</publicIp>" + extra + tags + "</item>"
}

func igwItem(id, owner, vpc string) string {
	att := ""
	if vpc != "" {
		att = "<attachmentSet><item><vpcId>" + vpc + "</vpcId><state>available</state></item></attachmentSet>"
	}
	tags := ""
	if owner != "" {
		tags = tagXML(owner)
	}
	return "<item><internetGatewayId>" + id + "</internetGatewayId>" + att + tags + "</item>"
}

func vpcItem(id, owner string) string {
	tags := ""
	if owner != "" {
		tags = tagXML(owner)
	}
	return "<item><vpcId>" + id + "</vpcId><state>available</state><cidrBlock>10.0.0.0/16</cidrBlock>" +
		"<ownerId>123456789012</ownerId><isDefault>false</isDefault>" + tags + "</item>"
}

func snapItem(id, owner string) string {
	desc := strings.ToLower(owner) + "-snap"
	return "<item><snapshotId>" + id + "</snapshotId><volumeId>vol-src</volumeId><status>completed</status>" +
		"<ownerId>123456789012</ownerId><description>" + desc + "</description>" + tagXML(owner) + "</item>"
}

func keyItem(name string) string {
	return "<item><keyName>" + name + "</keyName><keyFingerprint>ab</keyFingerprint></item>"
}

func subnetItem(id, owner string) string {
	return "<item><subnetId>" + id + "</subnetId><state>available</state><vpcId>vpc-x</vpcId>" +
		"<cidrBlock>10.0.1.0/24</cidrBlock><availabilityZone>vws-1a</availabilityZone>" +
		tagXML(owner) + "</item>"
}
