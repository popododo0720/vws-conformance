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
	rt.Cleanup(del)
	return nil
}

func (rt *Runtime) Track(kind, id, name string) error {
	row := sweep.Resource{Kind: kind, ID: id, Name: name, LedgerRunID: rt.Cfg.RunID}
	if err := os.MkdirAll(rt.Cfg.OutDir, 0o755); err != nil {
		return fmt.Errorf("ledger dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(rt.Cfg.OutDir, "ledger.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	defer f.Close()
	b, err := json.Marshal(row)
	if err != nil {
		return err
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
			return "", "", ctx.Err()
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
		_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteVpcOutput, error) {
			return rt.Clients.EC2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(vpcID)})
		})
		return IgnoreCleanup(err)
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
		_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSubnetOutput, error) {
			return rt.Clients.EC2.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subID)})
		})
		return IgnoreCleanup(err)
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
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return err
	}
	var picked []sweep.Resource
	for _, r := range rows {
		if rt.keepRun(r.LedgerRunID) {
			picked = append(picked, r)
		}
	}
	items, err := rt.inventory(ctx, picked)
	if err != nil {
		return err
	}
	if rt.Dry {
		return reportDry(items)
	}
	return rt.clearOwned(ctx, picked)
}

var sweepPause = 200 * time.Millisecond

func (rt *Runtime) clearOwned(ctx context.Context, rows []sweep.Resource) error {
	ctx, cancel := context.WithTimeout(ctx, OpTimeout)
	defer cancel()
	var last []sweepItem
	for {
		targets, err := rt.inventory(ctx, rows)
		if err != nil {
			return sweepGiveUp(ctx, last, err)
		}
		live, err := rt.liveInventory(ctx, rows)
		if err != nil {
			return sweepGiveUp(ctx, last, err)
		}
		if len(live) == 0 {
			if len(targets) == 0 {
				return nil
			}
			return rt.deleteItems(ctx, targets)
		}
		last = live
		if err := rt.deleteItems(ctx, targets); err != nil {
			return sweepGiveUp(ctx, last, err)
		}
		timer := time.NewTimer(sweepPause)
		select {
		case <-ctx.Done():
			timer.Stop()
			return leftoverError(last)
		case <-timer.C:
		}
	}
}

func sweepGiveUp(ctx context.Context, last []sweepItem, err error) error {
	if len(last) > 0 && ctx.Err() != nil {
		return leftoverError(last)
	}
	return err
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

func emptyAndDelete(ctx context.Context, c *s3.Client, name string) error {
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
			_, err := withDeadline(ctx, func(ctx context.Context) (*s3.DeleteObjectOutput, error) {
				return c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: obj.Key})
			})
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
	if err := deleteVersions(ctx, c, name); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*s3.DeleteBucketOutput, error) {
		return c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	})
	return ignoreMissing(err)
}

func deleteVersions(ctx context.Context, c *s3.Client, name string) error {
	var keyMarker, versionMarker *string
	seen := map[string]bool{}
	for {
		page, err := withDeadline(ctx, func(ctx context.Context) (*s3.ListObjectVersionsOutput, error) {
			return c.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
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
			if err := deleteOneVersion(ctx, c, name, obj.Key, obj.VersionId); err != nil {
				return err
			}
		}
		for _, obj := range page.DeleteMarkers {
			if err := deleteOneVersion(ctx, c, name, obj.Key, obj.VersionId); err != nil {
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

func deleteOneVersion(ctx context.Context, c *s3.Client, name string, key, version *string) error {
	_, err := withDeadline(ctx, func(ctx context.Context) (*s3.DeleteObjectOutput, error) {
		return c.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(name), Key: key, VersionId: version,
		})
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
	kind  string
	id    string
	assoc string
	vpcs  []string
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
				if ownedName(name, rt.keepRun) {
					items = append(items, sweepItem{kind: "bucket", id: name})
				}
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
		items = append(items, collectEC2(view, rows, ghosts, rt.keepRun)...)
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
			return nil, nil
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
	return collectEC2(view, rows, true, validRun)
}

func collectEC2(view ec2View, rows []sweep.Resource, ghosts bool, keep func(string) bool) []sweepItem {
	var items []sweepItem
	seen := map[string]bool{}
	add := func(it sweepItem) {
		key := it.kind + " " + it.id
		if it.id == "" || seen[key] {
			return
		}
		seen[key] = true
		items = append(items, it)
	}
	for _, in := range view.instances {
		if in.State != nil && in.State.Name == ec2types.InstanceStateNameTerminated {
			continue
		}
		id := aws.ToString(in.InstanceId)
		if ownedTag(in.Tags, keep) || ledgerHas(rows, "instance", id, keep) {
			add(sweepItem{kind: "instance", id: id})
		}
	}
	for _, v := range view.volumes {
		id := aws.ToString(v.VolumeId)
		if ledgerHas(rows, "volume", id, keep) || ownedTag(v.Tags, keep) {
			add(sweepItem{kind: "volume", id: id})
		}
	}
	if ghosts {
		for _, row := range rows {
			if row.Kind == "volume" && keep(row.LedgerRunID) {
				add(sweepItem{kind: "volume", id: row.ID})
			}
		}
	}
	for _, s := range view.snapshots {
		id := aws.ToString(s.SnapshotId)
		desc := aws.ToString(s.Description)
		if ownedName(desc, keep) || ledgerHas(rows, "snapshot", id, keep) {
			add(sweepItem{kind: "snapshot", id: id})
		}
	}
	for _, a := range view.addresses {
		id := aws.ToString(a.AllocationId)
		if ledgerHas(rows, "address", id, keep) || ownedTag(a.Tags, keep) {
			add(sweepItem{kind: "address", id: id, assoc: aws.ToString(a.AssociationId)})
		}
	}
	if ghosts {
		for _, row := range rows {
			if row.Kind == "address" && keep(row.LedgerRunID) {
				add(sweepItem{kind: "address", id: row.ID})
			}
		}
	}
	for _, k := range view.keys {
		name := aws.ToString(k.KeyName)
		if ownedName(name, keep) {
			add(sweepItem{kind: "key-pair", id: name})
		}
	}
	for _, g := range view.groups {
		if ownedName(aws.ToString(g.GroupName), keep) {
			add(sweepItem{kind: "security-group", id: aws.ToString(g.GroupId)})
		}
	}
	for _, s := range view.subnets {
		if ownedTag(s.Tags, keep) {
			add(sweepItem{kind: "subnet", id: aws.ToString(s.SubnetId)})
		}
	}
	for _, g := range view.gateways {
		if !ownedTag(g.Tags, keep) {
			continue
		}
		var vpcs []string
		for _, a := range g.Attachments {
			if id := aws.ToString(a.VpcId); id != "" {
				vpcs = append(vpcs, id)
			}
		}
		add(sweepItem{kind: "internet-gateway", id: aws.ToString(g.InternetGatewayId), vpcs: vpcs})
	}
	for _, v := range view.vpcs {
		if ownedTag(v.Tags, keep) {
			add(sweepItem{kind: "vpc", id: aws.ToString(v.VpcId)})
		}
	}
	return items
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

func (rt *Runtime) deleteItems(ctx context.Context, items []sweepItem) error {
	for _, item := range items {
		if item.kind != "bucket" || rt.Clients == nil || rt.Clients.S3 == nil {
			continue
		}
		if err := emptyAndDelete(ctx, rt.Clients.S3, item.id); err != nil {
			return err
		}
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return nil
	}
	return deleteEC2(ctx, rt, items)
}

func deleteEC2(ctx context.Context, rt *Runtime, items []sweepItem) error {
	c := rt.Clients.EC2
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
		_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.TerminateInstancesOutput, error) {
			return c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids[:n]})
		})
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("terminate: %w", err)
		}
		ids = ids[n:]
	}
	for _, item := range items {
		var err error
		switch item.kind {
		case "volume":
			_, _ = withDeadline(ctx, func(ctx context.Context) (*ec2.DetachVolumeOutput, error) {
				return c.DetachVolume(ctx, &ec2.DetachVolumeInput{VolumeId: aws.String(item.id)})
			})
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteVolumeOutput, error) {
				return c.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "IncorrectState") {
				return fmt.Errorf("delete volume %s: %w", item.id, err)
			}
		case "snapshot":
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSnapshotOutput, error) {
				return c.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete snapshot: %w", err)
			}
		case "address":
			if item.assoc != "" {
				_, _ = withDeadline(ctx, func(ctx context.Context) (*ec2.DisassociateAddressOutput, error) {
					return c.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{AssociationId: aws.String(item.assoc)})
				})
			}
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.ReleaseAddressOutput, error) {
				return c.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("release address: %w", err)
			}
		case "key-pair":
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteKeyPairOutput, error) {
				return c.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete key pair: %w", err)
			}
		case "security-group":
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSecurityGroupOutput, error) {
				return c.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete security group: %w", err)
			}
		case "internet-gateway":
			for _, vpc := range item.vpcs {
				_, _ = withDeadline(ctx, func(ctx context.Context) (*ec2.DetachInternetGatewayOutput, error) {
					return c.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{InternetGatewayId: aws.String(item.id), VpcId: aws.String(vpc)})
				})
			}
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteInternetGatewayOutput, error) {
				return c.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{InternetGatewayId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete internet gateway: %w", err)
			}
		case "subnet":
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSubnetOutput, error) {
				return c.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "Dependency") {
				return fmt.Errorf("delete subnet: %w", err)
			}
		case "vpc":
			_, err = withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteVpcOutput, error) {
				return c.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(item.id)})
			})
			if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "Dependency") {
				return fmt.Errorf("delete vpc: %w", err)
			}
		}
	}
	return nil
}

func ledgerHas(rows []sweep.Resource, kind, id string, keep func(string) bool) bool {
	for _, r := range rows {
		if r.Kind == kind && r.ID == id && keep(r.LedgerRunID) {
			return true
		}
	}
	return false
}
