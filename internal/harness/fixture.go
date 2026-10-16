package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

type netIDs struct {
	vpc    string
	subnet string
}

type netFlight struct {
	done        chan struct{}
	vpc, subnet string
	err         error
}

func (rt *Runtime) Tags() []ec2types.Tag {
	return []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String(NamePrefix(rt.Cfg.RunID))},
		{Key: aws.String("vws:conformance"), Value: aws.String("true")},
		{Key: aws.String("vws:conformance-run"), Value: aws.String(rt.Cfg.RunID)},
	}
}

func (rt *Runtime) TagSpec(kind ec2types.ResourceType) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{{ResourceType: kind, Tags: rt.Tags()}}
}

func (rt *Runtime) Token(suffix string) string {
	s := ResourceName(rt.Cfg.RunID, suffix)
	s = strings.ReplaceAll(s, "conf-", "c")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func (rt *Runtime) Own(kind, id, name string, del func(context.Context) error) error {
	if err := rt.Track(kind, id, name); err != nil {
		return err
	}
	rt.Cleanup(func(ctx context.Context) error {
		return rt.Keep(ctx, true, nil, nil, del)
	})
	return nil
}

func (rt *Runtime) Track(kind, id, name string) error {
	row := sweep.Resource{
		Kind: kind, ID: id, Name: name, LedgerRunID: rt.Cfg.RunID,
		Target: rt.Cfg.Target, Account: rt.Cfg.AccountID,
	}
	if err := os.MkdirAll(rt.Cfg.OutDir, 0o755); err != nil {
		return fmt.Errorf("ledger dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(rt.Cfg.OutDir, "ledger.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	defer closeChecked(f)
	b, err := json.Marshal(row)
	if err != nil {
		return wrapErr("Track", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("ledger write: %w", err)
	}
	return nil
}

func (rt *Runtime) Identify(ctx context.Context) (string, error) {
	if rt.Clients.STS == nil {
		return "", fmt.Errorf("IAM endpoint is unset")
	}
	out, err := withDeadline(ctx, func(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
		return rt.Clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	})
	if err != nil {
		return "", fmt.Errorf("GetCallerIdentity: %w", err)
	}
	return aws.ToString(out.Account), nil
}

func (rt *Runtime) BindAccount(account string) error {
	if account == "" {
		return fmt.Errorf("verified account is empty")
	}
	if rt.Cfg.AccountID != "" && rt.Cfg.AccountID != account {
		return fmt.Errorf("configured account %s does not match verified account %s", rt.Cfg.AccountID, account)
	}
	rt.Cfg.AccountID = account
	return nil
}

func (rt *Runtime) EnsureNet(ctx context.Context) (string, string, error) {
	rt.netMu.Lock()
	if rt.net.vpc != "" {
		vpc, subnet := rt.net.vpc, rt.net.subnet
		rt.netMu.Unlock()
		return vpc, subnet, nil
	}
	if rt.netFlight != nil {
		flight := rt.netFlight
		rt.netMu.Unlock()
		select {
		case <-ctx.Done():
			return "", "", wrapErr("EnsureNet", ctx.Err())
		case <-flight.done:
			return flight.vpc, flight.subnet, flight.err
		}
	}
	flight := &netFlight{done: make(chan struct{})}
	rt.netFlight = flight
	rt.netMu.Unlock()

	vpc, subnet, err := rt.createNet(ctx)
	rt.netMu.Lock()
	if err == nil {
		rt.net = netIDs{vpc: vpc, subnet: subnet}
	}
	flight.vpc, flight.subnet, flight.err = vpc, subnet, err
	rt.netFlight = nil
	close(flight.done)
	rt.netMu.Unlock()
	return vpc, subnet, err
}

func (rt *Runtime) createNet(ctx context.Context) (string, string, error) {
	if rt.Clients.EC2 == nil {
		return "", "", fmt.Errorf("EC2 endpoint is unset")
	}
	vpcOut, err := withDeadline(ctx, func(ctx context.Context) (*ec2.CreateVpcOutput, error) {
		return rt.Clients.EC2.CreateVpc(ctx, &ec2.CreateVpcInput{
			CidrBlock:         aws.String("10.0.0.0/16"),
			TagSpecifications: rt.TagSpec(ec2types.ResourceTypeVpc),
		})
	})
	if err != nil {
		return "", "", fmt.Errorf("create vpc: %w", err)
	}
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)
	if err := rt.Track("vpc", vpcID, rt.Name("vpc")); err != nil {
		return "", "", err
	}
	rt.Cleanup(func(ctx context.Context) error {
		return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
			return IgnoreCleanup(rt.EC2DeleteVpc(ctx, vpcID))
		})
	})
	subOut, err := withDeadline(ctx, func(ctx context.Context) (*ec2.CreateSubnetOutput, error) {
		return rt.Clients.EC2.CreateSubnet(ctx, &ec2.CreateSubnetInput{
			VpcId:             aws.String(vpcID),
			CidrBlock:         aws.String("10.0.1.0/24"),
			AvailabilityZone:  aws.String("vws-1a"),
			TagSpecifications: rt.TagSpec(ec2types.ResourceTypeSubnet),
		})
	})
	if err != nil {
		return "", "", fmt.Errorf("create subnet: %w", err)
	}
	subID := aws.ToString(subOut.Subnet.SubnetId)
	if err := rt.Track("subnet", subID, rt.Name("sub")); err != nil {
		return "", "", err
	}
	rt.Cleanup(func(ctx context.Context) error {
		return rt.Keep(ctx, true, nil, nil, func(ctx context.Context) error {
			return IgnoreCleanup(rt.EC2DeleteSubnet(ctx, subID))
		})
	})
	return vpcID, subID, nil
}

func ignoreMissing(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	if strings.Contains(s, "NotFound") || strings.Contains(s, "NoSuch") {
		return nil
	}
	return err
}

func IgnoreCleanup(err error) error {
	if ignoreMissing(err) == nil {
		return nil
	}
	s := err.Error()
	if strings.Contains(s, "Dependency") || strings.Contains(s, "IncorrectState") {
		return nil
	}
	return err
}

func (rt *Runtime) Sweep(ctx context.Context) error {
	rt.devMu.Lock()
	rt.swept = nil
	rt.devMu.Unlock()
	if rt.Cfg.AccountID == "" {
		return fmt.Errorf("ledger account is empty")
	}
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return err
	}
	items, err := rt.inventory(ctx, rows)
	if err != nil {
		return err
	}
	if rt.Dry {
		return reportDry(items)
	}
	return rt.clearOwned(ctx, rows)
}

var sweepPause = 200 * time.Millisecond

func (rt *Runtime) clearOwned(ctx context.Context, rows []sweep.Resource) error {
	ctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	var last []sweepItem
	for {
		targets, err := rt.inventory(ctx, rows)
		if err != nil {
			return rt.sweepGiveUp(ctx, last, err)
		}
		live, err := rt.liveInventory(ctx, rows)
		if err != nil {
			return rt.sweepGiveUp(ctx, last, err)
		}
		if len(live) == 0 {
			if len(targets) == 0 {
				return nil
			}
			return rt.deleteItems(ctx, targets)
		}
		last = live
		if err := rt.deleteItems(ctx, targets); err != nil && !retryableDelete(err) {
			return rt.sweepGiveUp(ctx, last, err)
		}
		timer := time.NewTimer(sweepPause)
		select {
		case <-ctx.Done():
			timer.Stop()
			return rt.leftover(last)
		case <-timer.C:
		}
	}
}

func (rt *Runtime) sweepGiveUp(ctx context.Context, last []sweepItem, err error) error {
	if len(last) > 0 && ctx.Err() != nil {
		return rt.leftover(last)
	}
	return err
}

// ForeignLeftoverError is a sweep that still sees resources owned by a finished run
// other than the caller. The caller's own leftovers stay a plain error.
type ForeignLeftoverError struct {
	msg string
}

func (e *ForeignLeftoverError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

func (rt *Runtime) leftover(items []sweepItem) error {
	base := leftoverError(items)
	if foreignOnly(items, rt.Cfg.RunID) {
		return &ForeignLeftoverError{msg: base.Error()}
	}
	return base
}

func foreignOnly(items []sweepItem, current string) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if len(item.owners) == 0 {
			return false
		}
		for _, owner := range item.owners {
			if owner == current {
				return false
			}
		}
	}
	return true
}

func leftoverError(items []sweepItem) error {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.kind+" "+item.id)
	}
	return fmt.Errorf("sweep left %s", strings.Join(parts, ", "))
}

func readLedger(dir string) ([]sweep.Resource, error) {
	b, err := os.ReadFile(filepath.Join(dir, "ledger.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger: %w", err)
	}
	var out []sweep.Resource
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r sweep.Resource
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, fmt.Errorf("ledger line: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

func (rt *Runtime) ClaimBucket(ctx context.Context, name string, created bool) error {
	var owners []string
	if !created {
		tags, err := rt.bucketTags(ctx, name)
		if err != nil {
			return err
		}
		owners = tagOwners(tags)
	}
	keep := func(id string) bool { return id == rt.Cfg.RunID }
	if !AllowMutation(created, owners, nil, keep) {
		return fmt.Errorf("preserve bucket %s", name)
	}
	return nil
}

func (rt *Runtime) bucketTags(ctx context.Context, name string) ([]ec2types.Tag, error) {
	out, err := withDeadline(ctx, func(ctx context.Context) (*s3.GetBucketTaggingOutput, error) {
		return rt.Clients.S3.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{Bucket: aws.String(name)})
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchTagSet") {
			return nil, nil
		}
		return nil, fmt.Errorf("get bucket tagging %s: %w", name, err)
	}
	var tags []ec2types.Tag
	if out != nil {
		for _, tag := range out.TagSet {
			tags = append(tags, ec2types.Tag{Key: tag.Key, Value: tag.Value})
		}
	}
	return tags, nil
}

func emptyAndDelete(ctx context.Context, rt *Runtime, name string) error {
	c := rt.Clients.S3
	var token *string
	for {
		page, err := withDeadline(ctx, func(ctx context.Context) (*s3.ListObjectsV2Output, error) {
			return c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(name), ContinuationToken: token})
		})
		if err != nil {
			if strings.Contains(err.Error(), "NoSuchBucket") {
				return nil
			}
			return fmt.Errorf("list %s: %w", name, err)
		}
		for _, obj := range page.Contents {
			_, err := rt.S3DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: obj.Key})
			if err != nil {
				return fmt.Errorf("delete object: %w", err)
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		next := aws.ToString(page.NextContinuationToken)
		if next == "" || next == aws.ToString(token) {
			return fmt.Errorf("repeated pagination token %q", next)
		}
		token = page.NextContinuationToken
	}
	if err := deleteVersions(ctx, rt, name); err != nil {
		return err
	}
	return ignoreMissing(rt.S3DeleteBucket(ctx, name))
}

func deleteVersions(ctx context.Context, rt *Runtime, name string) error {
	var keyMarker, versionMarker *string
	seen := map[string]bool{}
	for {
		page, err := withDeadline(ctx, func(ctx context.Context) (*s3.ListObjectVersionsOutput, error) {
			return rt.Clients.S3.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
				Bucket: aws.String(name), KeyMarker: keyMarker, VersionIdMarker: versionMarker,
			})
		})
		if err != nil {
			if strings.Contains(err.Error(), "NoSuchBucket") || versionListUnsupported(err) {
				return nil
			}
			return fmt.Errorf("list versions %s: %w", name, err)
		}
		for _, obj := range page.Versions {
			if err := deleteOneVersion(ctx, rt, name, obj.Key, obj.VersionId); err != nil {
				return err
			}
		}
		for _, obj := range page.DeleteMarkers {
			if err := deleteOneVersion(ctx, rt, name, obj.Key, obj.VersionId); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			return nil
		}
		next := aws.ToString(page.NextKeyMarker) + "\n" + aws.ToString(page.NextVersionIdMarker)
		if next == "\n" || seen[next] {
			return fmt.Errorf("repeated pagination token %q", next)
		}
		seen[next] = true
		keyMarker = page.NextKeyMarker
		versionMarker = page.NextVersionIdMarker
	}
}

func deleteOneVersion(ctx context.Context, rt *Runtime, name string, key, version *string) error {
	_, err := rt.S3DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(name), Key: key, VersionId: version,
	})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func versionListUnsupported(err error) bool {
	s := err.Error()
	return strings.Contains(s, "NotImplemented") || strings.Contains(s, "not served") || strings.Contains(s, "501")
}

type sweepItem struct {
	kind   string
	id     string
	assoc  string
	vpcs   []string
	owners []string
	peers  [][]string
	direct bool
}

type ec2View struct {
	instances []ec2types.Instance
	volumes   []ec2types.Volume
	snapshots []ec2types.Snapshot
	addresses []ec2types.Address
	keys      []ec2types.KeyPairInfo
	groups    []ec2types.SecurityGroup
	subnets   []ec2types.Subnet
	vpcs      []ec2types.Vpc
	gateways  []ec2types.InternetGateway
}

func (rt *Runtime) inventory(ctx context.Context, rows []sweep.Resource) ([]sweepItem, error) {
	return rt.collectInventory(ctx, rows, true)
}

func (rt *Runtime) liveInventory(ctx context.Context, rows []sweep.Resource) ([]sweepItem, error) {
	return rt.collectInventory(ctx, rows, false)
}

func (rt *Runtime) collectInventory(ctx context.Context, rows []sweep.Resource, ghosts bool) ([]sweepItem, error) {
	var items []sweepItem
	if rt.Clients != nil && rt.Clients.S3 != nil {
		err := eachPage(func(token *string) (*string, error) {
			out, err := withDeadline(ctx, func(ctx context.Context) (*s3.ListBucketsOutput, error) {
				return rt.Clients.S3.ListBuckets(ctx, &s3.ListBucketsInput{ContinuationToken: token})
			})
			if err != nil {
				return nil, fmt.Errorf("list buckets: %w", err)
			}
			for _, b := range out.Buckets {
				name := aws.ToString(b.Name)
				if !ownedName(name, rt.keepRun) {
					continue
				}
				tags, tagErr := rt.bucketTags(ctx, name)
				if tagErr != nil {
					return nil, tagErr
				}
				if ownerOutsideKeep(tags, []string{name}, rt.keepRun) {
					continue
				}
				items = append(items, sweepItem{
					kind: "bucket", id: name, owners: mergeOwners(runOwners(name), tagOwners(tags)),
				})
			}
			return out.ContinuationToken, nil
		})
		if err != nil {
			return nil, err
		}
	}
	if rt.Clients != nil && rt.Clients.EC2 != nil {
		view, err := describeEC2(ctx, rt.Clients.EC2)
		if err != nil {
			return nil, err
		}
		items = append(items, collectEC2(view, rows, ghosts, rt.keepRun, rt.ledgerScope())...)
	}
	return items, nil
}

func describeEC2(ctx context.Context, c *ec2.Client) (ec2View, error) {
	var view ec2View
	err := eachPage(func(token *string) (*string, error) {
		inst, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeInstancesOutput, error) {
			return c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range inst.Reservations {
			view.instances = append(view.instances, res.Instances...)
		}
		return inst.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	err = eachPage(func(token *string) (*string, error) {
		vols, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeVolumesOutput, error) {
			return c.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe volumes: %w", err)
		}
		view.volumes = append(view.volumes, vols.Volumes...)
		return vols.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	err = eachPage(func(token *string) (*string, error) {
		snaps, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSnapshotsOutput, error) {
			return c.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}, NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe snapshots: %w", err)
		}
		view.snapshots = append(view.snapshots, snaps.Snapshots...)
		return snaps.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	addrs, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeAddressesOutput, error) {
		return c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	})
	if err != nil {
		return view, fmt.Errorf("describe addresses: %w", err)
	}
	view.addresses = addrs.Addresses
	keys, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeKeyPairsOutput, error) {
		return c.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{})
	})
	if err != nil {
		return view, fmt.Errorf("describe key pairs: %w", err)
	}
	view.keys = keys.KeyPairs
	err = eachPage(func(token *string) (*string, error) {
		groups, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSecurityGroupsOutput, error) {
			return c.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe security groups: %w", err)
		}
		view.groups = append(view.groups, groups.SecurityGroups...)
		return groups.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	err = eachPage(func(token *string) (*string, error) {
		subs, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSubnetsOutput, error) {
			return c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe subnets: %w", err)
		}
		view.subnets = append(view.subnets, subs.Subnets...)
		return subs.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	err = eachPage(func(token *string) (*string, error) {
		vpcs, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeVpcsOutput, error) {
			return c.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{NextToken: token})
		})
		if err != nil {
			return nil, fmt.Errorf("describe vpcs: %w", err)
		}
		view.vpcs = append(view.vpcs, vpcs.Vpcs...)
		return vpcs.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	err = eachPage(func(token *string) (*string, error) {
		gws, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeInternetGatewaysOutput, error) {
			return c.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{NextToken: token})
		})
		if unsupportedAction(err) {
			// An empty token ends the page walk. A nil token is an invalid value.
			return aws.String(""), nil
		}
		if err != nil {
			return nil, fmt.Errorf("describe internet gateways: %w", err)
		}
		view.gateways = append(view.gateways, gws.InternetGateways...)
		return gws.NextToken, nil
	})
	if err != nil {
		return view, err
	}
	return view, nil
}

func eachPage(fetch func(token *string) (*string, error)) error {
	seen := map[string]bool{}
	var token *string
	for {
		next, err := fetch(token)
		if err != nil {
			return err
		}
		n := aws.ToString(next)
		if n == "" {
			return nil
		}
		if seen[n] {
			return fmt.Errorf("repeated pagination token %q", n)
		}
		seen[n] = true
		token = aws.String(n)
	}
}

func ec2Items(view ec2View, rows []sweep.Resource) []sweepItem {
	return collectEC2(view, rows, true, validRun, ledgerScope{})
}

type ledgerScope struct {
	Target  string
	Account string
}

func (rt *Runtime) ledgerScope() ledgerScope {
	return ledgerScope{Target: rt.Cfg.Target, Account: rt.Cfg.AccountID}
}

func collectEC2(view ec2View, rows []sweep.Resource, ghosts bool, keep func(string) bool,
	scope ledgerScope,
) []sweepItem {
	var items []sweepItem
	seen := map[string]int{}
	blocked := map[string]bool{}
	instSeen := map[string]bool{}
	instOwners := map[string][]string{}
	for _, in := range view.instances {
		id := aws.ToString(in.InstanceId)
		if id == "" {
			continue
		}
		instSeen[id] = true
		instOwners[id] = resourceOwners(in.Tags, rows, "instance", id, scope)
	}
	vpcSeen := map[string]bool{}
	vpcOwners := map[string][]string{}
	for _, v := range view.vpcs {
		id := aws.ToString(v.VpcId)
		if id == "" {
			continue
		}
		vpcSeen[id] = true
		vpcOwners[id] = resourceOwners(v.Tags, rows, "vpc", id, scope)
	}
	instPeers := instanceVolumePeers(view.volumes, rows, scope)
	add := func(it sweepItem) {
		key := it.kind + " " + it.id
		if it.id == "" {
			return
		}
		if i, ok := seen[key]; ok {
			items[i].owners = mergeOwners(items[i].owners, it.owners)
			if items[i].assoc == "" {
				items[i].assoc = it.assoc
			}
			if len(items[i].vpcs) == 0 {
				items[i].vpcs = it.vpcs
			}
			if len(items[i].peers) == 0 {
				items[i].peers = it.peers
			}
			return
		}
		seen[key] = len(items)
		items = append(items, it)
	}
	for _, in := range view.instances {
		if in.State != nil && in.State.Name == ec2types.InstanceStateNameTerminated {
			continue
		}
		id := aws.ToString(in.InstanceId)
		if ownerOutsideKeep(in.Tags, nil, keep) {
			continue
		}
		if ownedTag(in.Tags, keep) || ledgerHas(rows, "instance", id, keep, scope) {
			add(sweepItem{
				kind: "instance", id: id,
				owners: resourceOwners(in.Tags, rows, "instance", id, scope),
				peers:  instPeers[id],
			})
		}
	}
	for _, v := range view.volumes {
		id := aws.ToString(v.VolumeId)
		if ownerOutsideKeep(v.Tags, nil, keep) {
			blocked["volume "+id] = true
			continue
		}
		if ledgerHas(rows, "volume", id, keep, scope) || ownedTag(v.Tags, keep) {
			add(sweepItem{
				kind: "volume", id: id,
				owners: resourceOwners(v.Tags, rows, "volume", id, scope),
				peers:  volumePeers(v, instSeen, instOwners),
			})
		}
	}
	if ghosts {
		for _, row := range rows {
			if row.Kind == "volume" && keep(row.LedgerRunID) && rowInScope(row, scope) && !blocked["volume "+row.ID] {
				add(sweepItem{
					kind: "volume", id: row.ID, direct: true,
					owners: ledgerOwners(rows, "volume", row.ID, scope),
				})
			}
		}
	}
	for _, s := range view.snapshots {
		id := aws.ToString(s.SnapshotId)
		desc := aws.ToString(s.Description)
		if ownerOutsideKeep(s.Tags, []string{desc}, keep) {
			continue
		}
		if ownedName(desc, keep) || ledgerHas(rows, "snapshot", id, keep, scope) {
			add(sweepItem{
				kind: "snapshot", id: id,
				owners: resourceOwners(s.Tags, rows, "snapshot", id, scope, desc),
			})
		}
	}
	for _, a := range view.addresses {
		id := aws.ToString(a.AllocationId)
		if ownerOutsideKeep(a.Tags, nil, keep) {
			blocked["address "+id] = true
			continue
		}
		if ledgerHas(rows, "address", id, keep, scope) || ownedTag(a.Tags, keep) {
			add(sweepItem{
				kind:   "address",
				id:     id,
				assoc:  aws.ToString(a.AssociationId),
				owners: resourceOwners(a.Tags, rows, "address", id, scope),
				peers:  addressPeers(a, instSeen, instOwners),
			})
		}
	}
	if ghosts {
		for _, row := range rows {
			if row.Kind == "address" && keep(row.LedgerRunID) && rowInScope(row, scope) && !blocked["address "+row.ID] {
				add(sweepItem{
					kind: "address", id: row.ID, direct: true,
					owners: ledgerOwners(rows, "address", row.ID, scope),
				})
			}
		}
	}
	for _, k := range view.keys {
		name := aws.ToString(k.KeyName)
		if ownerOutsideKeep(k.Tags, []string{name}, keep) {
			continue
		}
		if ownedName(name, keep) {
			add(sweepItem{
				kind: "key-pair", id: name,
				owners: resourceOwners(k.Tags, rows, "key-pair", name, scope, name),
			})
		}
	}
	for _, g := range view.groups {
		name := aws.ToString(g.GroupName)
		if ownerOutsideKeep(g.Tags, []string{name}, keep) {
			continue
		}
		if ownedName(name, keep) {
			add(sweepItem{
				kind:   "security-group",
				id:     aws.ToString(g.GroupId),
				owners: resourceOwners(g.Tags, rows, "security-group", aws.ToString(g.GroupId), scope, name),
			})
		}
	}
	for _, s := range view.subnets {
		if ownerOutsideKeep(s.Tags, nil, keep) {
			continue
		}
		if ownedTag(s.Tags, keep) {
			id := aws.ToString(s.SubnetId)
			add(sweepItem{kind: "subnet", id: id, owners: resourceOwners(s.Tags, rows, "subnet", id, scope)})
		}
	}
	for _, g := range view.gateways {
		if ownerOutsideKeep(g.Tags, nil, keep) || !ownedTag(g.Tags, keep) {
			continue
		}
		var vpcs []string
		for _, a := range g.Attachments {
			if id := aws.ToString(a.VpcId); id != "" {
				vpcs = append(vpcs, id)
			}
		}
		id := aws.ToString(g.InternetGatewayId)
		add(sweepItem{
			kind:   "internet-gateway",
			id:     id,
			vpcs:   vpcs,
			owners: resourceOwners(g.Tags, rows, "internet-gateway", id, scope),
			peers:  gatewayPeers(g, vpcSeen, vpcOwners),
		})
	}
	for _, v := range view.vpcs {
		if ownerOutsideKeep(v.Tags, nil, keep) {
			continue
		}
		if ownedTag(v.Tags, keep) {
			id := aws.ToString(v.VpcId)
			add(sweepItem{kind: "vpc", id: id, owners: resourceOwners(v.Tags, rows, "vpc", id, scope)})
		}
	}
	return items
}

func ledgerOwners(rows []sweep.Resource, kind, id string, scope ledgerScope) []string {
	var owners []string
	for _, row := range rows {
		if row.Kind != kind || row.ID != id || !rowInScope(row, scope) {
			continue
		}
		if ValidateRunID(row.LedgerRunID) != nil {
			owners = appendUnverified(owners)
			continue
		}
		owners = appendOwner(owners, row.LedgerRunID)
	}
	return owners
}

func resourceOwners(tags []ec2types.Tag, rows []sweep.Resource, kind, id string, scope ledgerScope,
	names ...string,
) []string {
	owners := mergeOwners(tagOwners(tags), ledgerOwners(rows, kind, id, scope))
	for _, name := range names {
		owners = mergeOwners(owners, runOwners(name))
	}
	return owners
}

func unsupportedAction(err error) bool {
	return err != nil && strings.Contains(err.Error(), "InvalidAction")
}

func reportDry(items []sweepItem) error {
	for _, item := range items {
		fmt.Println("leftover", item.kind, item.id)
	}
	if len(items) > 0 {
		return fmt.Errorf("%d leftover resources", len(items))
	}
	fmt.Println("leftovers 0")
	return nil
}

func peerGroup(id string, seen map[string]bool, owners map[string][]string) []string {
	if id == "" || !seen[id] {
		return []string{}
	}
	return append([]string{}, owners[id]...)
}

func volumePeers(v ec2types.Volume, seen map[string]bool, owners map[string][]string) [][]string {
	var peers [][]string
	for _, a := range v.Attachments {
		if a.State == ec2types.VolumeAttachmentStateDetached {
			continue
		}
		inst := aws.ToString(a.InstanceId)
		if inst == "" {
			peers = append(peers, []string{})
			continue
		}
		peers = append(peers, peerGroup(inst, seen, owners))
	}
	return peers
}

func addressPeers(a ec2types.Address, seen map[string]bool, owners map[string][]string) [][]string {
	inst := aws.ToString(a.InstanceId)
	if aws.ToString(a.AssociationId) == "" && inst == "" {
		return nil
	}
	return [][]string{peerGroup(inst, seen, owners)}
}

func gatewayPeers(g ec2types.InternetGateway, seen map[string]bool, owners map[string][]string) [][]string {
	var peers [][]string
	for _, a := range g.Attachments {
		vpc := aws.ToString(a.VpcId)
		if vpc == "" {
			peers = append(peers, []string{})
			continue
		}
		peers = append(peers, peerGroup(vpc, seen, owners))
	}
	return peers
}

func instanceVolumePeers(volumes []ec2types.Volume, rows []sweep.Resource, scope ledgerScope) map[string][][]string {
	out := map[string][][]string{}
	for _, v := range volumes {
		id := aws.ToString(v.VolumeId)
		owners := resourceOwners(v.Tags, rows, "volume", id, scope)
		for _, a := range v.Attachments {
			if a.State == ec2types.VolumeAttachmentStateDetached {
				continue
			}
			inst := aws.ToString(a.InstanceId)
			if inst == "" {
				continue
			}
			if len(owners) == 0 {
				out[inst] = append(out[inst], []string{})
				continue
			}
			out[inst] = append(out[inst], append([]string{}, owners...))
		}
	}
	return out
}

func (rt *Runtime) Keep(
	ctx context.Context, created bool, owners []string, peers [][]string, mutate func(context.Context) error,
) error {
	if !AllowMutation(created, owners, peers, rt.keepRun) {
		return fmt.Errorf("preserve unverified")
	}
	return mutate(ctx)
}

func (rt *Runtime) NoteDevice(id string) error {
	if id == "" {
		return nil
	}
	rt.devMu.Lock()
	if rt.devices == nil {
		rt.devices = map[string]struct{}{}
	}
	rt.devices[id] = struct{}{}
	rt.devMu.Unlock()
	return rt.Track("volume", id, rt.Name("root"))
}

func (rt *Runtime) notedDevice(id string) bool {
	rt.devMu.Lock()
	defer rt.devMu.Unlock()
	_, ok := rt.devices[id]
	return ok
}

func (rt *Runtime) TerminateOwned(ctx context.Context, id string) error {
	return IgnoreCleanup(rt.EC2Terminate(ctx, []string{id}))
}

func (rt *Runtime) DeleteOwnedVolume(ctx context.Context, id string) error {
	return IgnoreCleanup(rt.EC2DeleteVolume(ctx, id))
}

func (rt *Runtime) ReleaseOwned(ctx context.Context, id string) error {
	return IgnoreCleanup(rt.EC2ReleaseAddress(ctx, id))
}

func (rt *Runtime) DeleteOwnedKey(ctx context.Context, name string) error {
	return IgnoreCleanup(rt.EC2DeleteKeyPair(ctx, name))
}

func (rt *Runtime) instanceAttachmentPeers(ctx context.Context, instanceID string) ([][]string, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil, fmt.Errorf("ec2 client is unset")
	}
	var volumes []ec2types.Volume
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeVolumesOutput, error) {
			return rt.Clients.EC2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{NextToken: token})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe volumes returned nothing")
		}
		volumes = append(volumes, out.Volumes...)
		return out.NextToken, nil
	})
	if err != nil {
		return nil, err
	}
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return nil, err
	}
	var peers [][]string
	for _, v := range volumes {
		volID := aws.ToString(v.VolumeId)
		owners := resourceOwners(v.Tags, rows, "volume", volID, rt.ledgerScope())
		if rt.notedDevice(volID) {
			owners = appendOwner(owners, rt.Cfg.RunID)
		}
		for _, a := range v.Attachments {
			if a.State == ec2types.VolumeAttachmentStateDetached || aws.ToString(a.InstanceId) != instanceID {
				continue
			}
			if len(owners) == 0 {
				peers = append(peers, []string{})
				continue
			}
			peers = append(peers, append([]string{}, owners...))
		}
	}
	return peers, nil
}

func (rt *Runtime) markSwept(id string) {
	rt.devMu.Lock()
	defer rt.devMu.Unlock()
	if rt.swept == nil {
		rt.swept = map[string]struct{}{}
	}
	rt.swept[id] = struct{}{}
}

func (rt *Runtime) wasSwept(id string) bool {
	rt.devMu.Lock()
	defer rt.devMu.Unlock()
	_, ok := rt.swept[id]
	return ok
}

func (rt *Runtime) observe(ctx context.Context, item sweepItem) (sweepItem, bool, error) {
	if rt.wasSwept(item.id) {
		return item, true, nil
	}
	switch item.kind {
	case "volume":
		return rt.observeVolume(ctx, item)
	case "address":
		return rt.observeAddress(ctx, item)
	default:
		return item, false, fmt.Errorf("preserve %s %s", item.kind, item.id)
	}
}

func (rt *Runtime) observeVolume(ctx context.Context, item sweepItem) (sweepItem, bool, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return item, false, fmt.Errorf("ec2 client is unset")
	}
	var matched []ec2types.Volume
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeVolumesOutput, error) {
			return rt.Clients.EC2.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
				VolumeIds: []string{item.id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe volumes returned nothing")
		}
		for _, v := range out.Volumes {
			if aws.ToString(v.VolumeId) == item.id {
				matched = append(matched, v)
			}
		}
		return out.NextToken, nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") && len(matched) == 0 {
			return item, true, nil
		}
		return item, false, err
	}
	if len(matched) == 0 {
		return item, false, fmt.Errorf("preserve volume %s", item.id)
	}
	if len(matched) != 1 {
		return item, false, fmt.Errorf("preserve volume %s: ambiguous", item.id)
	}
	v := matched[0]
	item.owners = mergeOwners(item.owners, tagOwners(v.Tags))
	var peers [][]string
	for _, a := range v.Attachments {
		if a.State == ec2types.VolumeAttachmentStateDetached {
			continue
		}
		inst := aws.ToString(a.InstanceId)
		if inst == "" {
			peers = append(peers, []string{})
			continue
		}
		peers = append(peers, rt.describedInstanceOwners(ctx, inst))
	}
	item.peers = peers
	item.direct = false
	return item, false, nil
}

func (rt *Runtime) observeAddress(ctx context.Context, item sweepItem) (sweepItem, bool, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return item, false, fmt.Errorf("ec2 client is unset")
	}
	out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeAddressesOutput, error) {
		return rt.Clients.EC2.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{AllocationIds: []string{item.id}})
	})
	var matched []ec2types.Address
	if err == nil && out != nil {
		for _, addr := range out.Addresses {
			if aws.ToString(addr.AllocationId) == item.id {
				matched = append(matched, addr)
			}
		}
	}
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") && len(matched) == 0 {
			return item, true, nil
		}
		return item, false, err
	}
	if len(matched) == 0 {
		return item, false, fmt.Errorf("preserve address %s", item.id)
	}
	if len(matched) != 1 {
		return item, false, fmt.Errorf("preserve address %s: ambiguous", item.id)
	}
	addr := matched[0]
	item.assoc = aws.ToString(addr.AssociationId)
	item.owners = mergeOwners(item.owners, tagOwners(addr.Tags))
	inst := aws.ToString(addr.InstanceId)
	if item.assoc == "" && inst == "" {
		item.peers = nil
	} else {
		item.peers = [][]string{rt.describedInstanceOwners(ctx, inst)}
	}
	item.direct = false
	return item, false, nil
}

func (rt *Runtime) describedInstanceOwners(ctx context.Context, id string) []string {
	if id == "" || rt.Clients == nil || rt.Clients.EC2 == nil {
		return []string{}
	}
	matched, tags, err := rt.findInstance(ctx, id)
	rows, rowErr := readLedger(rt.Cfg.OutDir)
	if rowErr != nil {
		return []string{}
	}
	// This sweep already terminated the instance after the gate allowed it.
	// A later volume or address describe still names that instance.
	if rt.wasSwept(id) {
		return mergeOwners([]string{rt.Cfg.RunID}, ledgerOwners(rows, "instance", id, rt.ledgerScope()))
	}
	if err != nil || matched != 1 {
		if err != nil && strings.Contains(err.Error(), "NotFound") && matched == 0 {
			owners := ledgerOwners(rows, "instance", id, rt.ledgerScope())
			if AllowMutation(false, owners, nil, rt.keepRun) {
				return owners
			}
		}
		return []string{}
	}
	return mergeOwners(tagOwners(tags), ledgerOwners(rows, "instance", id, rt.ledgerScope()))
}

func (rt *Runtime) findInstance(ctx context.Context, id string) (int, []ec2types.Tag, error) {
	var matched int
	var tags []ec2types.Tag
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeInstancesOutput, error) {
			return rt.Clients.EC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
				InstanceIds: []string{id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe instances returned nothing")
		}
		for _, res := range out.Reservations {
			for _, inst := range res.Instances {
				if aws.ToString(inst.InstanceId) == id {
					matched++
					tags = inst.Tags
				}
			}
		}
		return out.NextToken, nil
	})
	return matched, tags, err
}

func (rt *Runtime) deleteItems(ctx context.Context, items []sweepItem) error {
	var safe []sweepItem
	var held []string
	for _, item := range items {
		if item.direct {
			observed, gone, err := rt.observe(ctx, item)
			if gone {
				continue
			}
			if err != nil {
				held = append(held, item.kind+" "+item.id)
				continue
			}
			item = observed
		}
		if AllowMutation(false, item.owners, item.peers, rt.keepRun) {
			safe = append(safe, item)
			continue
		}
		held = append(held, item.kind+" "+item.id)
	}
	var safeErr error
	for _, item := range safe {
		if item.kind != "bucket" || rt.Clients == nil || rt.Clients.S3 == nil {
			continue
		}
		if err := emptyAndDelete(ctx, rt, item.id); err != nil {
			safeErr = err
			break
		}
	}
	if safeErr == nil && rt.Clients != nil && rt.Clients.EC2 != nil {
		safeErr = deleteEC2(ctx, rt, safe)
	}
	if len(held) > 0 {
		if safeErr != nil && !retryableDelete(safeErr) {
			return safeErr
		}
		return fmt.Errorf("preserve %s", strings.Join(held, ", "))
	}
	return safeErr
}

func deleteEC2(ctx context.Context, rt *Runtime, items []sweepItem) error {
	var ids []string
	for _, item := range items {
		if item.kind == "instance" {
			ids = append(ids, item.id)
		}
	}
	for len(ids) > 0 {
		n := 20
		if len(ids) < n {
			n = len(ids)
		}
		err := rt.EC2Terminate(ctx, ids[:n])
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("terminate: %w", err)
		}
		for _, id := range ids[:n] {
			rt.markSwept(id)
		}
		ids = ids[n:]
	}
	for _, item := range items {
		var err error
		switch item.kind {
		case "volume":
			if detachErr := rt.EC2DetachVolume(ctx, item.id, "", ""); detachErr != nil {
				if blockedMutation(detachErr) {
					return detachErr
				}
				// Detach is best-effort. DeleteVolume is what decides the volume is gone.
				fmt.Fprintf(os.Stderr, "detach volume %s: %v\n", item.id, detachErr)
			}
			err = rt.EC2DeleteVolume(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete volume %s: %w", item.id, err)
			}
			rt.markSwept(item.id)
		case "snapshot":
			err = rt.EC2DeleteSnapshot(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete snapshot: %w", err)
			}
			rt.markSwept(item.id)
		case "address":
			if item.assoc != "" {
				if assocErr := rt.EC2DisassociateAddress(ctx, item.id, item.assoc); assocErr != nil {
					if blockedMutation(assocErr) {
						return assocErr
					}
					// Disassociate is best-effort. ReleaseAddress is what decides the address is gone.
					fmt.Fprintf(os.Stderr, "disassociate address %s: %v\n", item.id, assocErr)
				}
			}
			err = rt.EC2ReleaseAddress(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("release address: %w", err)
			}
			rt.markSwept(item.id)
		case "key-pair":
			err = rt.EC2DeleteKeyPair(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete key pair: %w", err)
			}
			rt.markSwept(item.id)
		case "security-group":
			err = rt.EC2DeleteSecurityGroup(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete security group: %w", err)
			}
			rt.markSwept(item.id)
		case "internet-gateway":
			for _, vpc := range item.vpcs {
				if gwErr := rt.EC2DetachInternetGateway(ctx, item.id, vpc); gwErr != nil {
					if blockedMutation(gwErr) {
						return gwErr
					}
					// Detach is best-effort. DeleteInternetGateway is what decides the gateway is gone.
					fmt.Fprintf(os.Stderr, "detach internet gateway %s: %v\n", item.id, gwErr)
				}
			}
			err = rt.EC2DeleteInternetGateway(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete internet gateway: %w", err)
			}
			rt.markSwept(item.id)
		case "subnet":
			err = rt.EC2DeleteSubnet(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete subnet: %w", err)
			}
			rt.markSwept(item.id)
		case "vpc":
			err = rt.EC2DeleteVpc(ctx, item.id)
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete vpc: %w", err)
			}
			rt.markSwept(item.id)
		}
	}
	return nil
}

func blockedMutation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "preserve") || strings.Contains(s, "ambiguous")
}

func retryableDelete(err error) bool {
	s := err.Error()
	return strings.Contains(s, "IncorrectState") || strings.Contains(s, "Dependency")
}

func rowInScope(row sweep.Resource, scope ledgerScope) bool {
	return row.Target == scope.Target && row.Account == scope.Account
}

func ledgerHas(rows []sweep.Resource, kind, id string, keep func(string) bool, scope ledgerScope) bool {
	for _, r := range rows {
		if r.Kind == kind && r.ID == id && keep(r.LedgerRunID) && rowInScope(r, scope) {
			return true
		}
	}
	return false
}
