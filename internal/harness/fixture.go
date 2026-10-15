package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	out, err := rt.Clients.STS.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("GetCallerIdentity: %w", err)
	}
	return aws.ToString(out.Account), nil
}

func (rt *Runtime) EnsureNet(ctx context.Context) (string, string, error) {
	if rt.net.vpc != "" {
		return rt.net.vpc, rt.net.subnet, nil
	}
	if rt.Clients.EC2 == nil {
		return "", "", fmt.Errorf("EC2 endpoint is unset")
	}
	vpcOut, err := rt.Clients.EC2.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock:         aws.String("10.0.0.0/16"),
		TagSpecifications: rt.TagSpec(ec2types.ResourceTypeVpc),
	})
	if err != nil {
		return "", "", fmt.Errorf("create vpc: %w", err)
	}
	vpcID := aws.ToString(vpcOut.Vpc.VpcId)
	if err := rt.Track("vpc", vpcID, rt.Name("vpc")); err != nil {
		return "", "", err
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := rt.Clients.EC2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(vpcID)})
		return ignoreMissing(err)
	})
	subOut, err := rt.Clients.EC2.CreateSubnet(ctx, &ec2.CreateSubnetInput{
		VpcId:             aws.String(vpcID),
		CidrBlock:         aws.String("10.0.1.0/24"),
		AvailabilityZone:  aws.String("vws-1a"),
		TagSpecifications: rt.TagSpec(ec2types.ResourceTypeSubnet),
	})
	if err != nil {
		return "", "", fmt.Errorf("create subnet: %w", err)
	}
	subID := aws.ToString(subOut.Subnet.SubnetId)
	if err := rt.Track("subnet", subID, rt.Name("sub")); err != nil {
		return "", "", err
	}
	rt.Cleanup(func(ctx context.Context) error {
		_, err := rt.Clients.EC2.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(subID)})
		return ignoreMissing(err)
	})
	rt.net = netIDs{vpc: vpcID, subnet: subID}
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

func (rt *Runtime) Sweep(ctx context.Context) error {
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return err
	}
	var picked []sweep.Resource
	for _, r := range rows {
		if sweep.CarriesPrefix(r) {
			picked = append(picked, r)
		}
	}
	if rt.Dry {
		return rt.drySweep(ctx, picked)
	}
	if rt.Clients.S3 != nil {
		if err := rt.sweepBuckets(ctx); err != nil {
			return err
		}
	}
	if rt.Clients.EC2 != nil {
		if err := rt.sweepEC2(ctx, picked); err != nil {
			return err
		}
	}
	return nil
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

func (rt *Runtime) sweepBuckets(ctx context.Context) error {
	out, err := rt.Clients.S3.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("list buckets: %w", err)
	}
	for _, b := range out.Buckets {
		name := aws.ToString(b.Name)
		if !strings.HasPrefix(name, "conf-") {
			continue
		}
		if err := emptyAndDelete(ctx, rt.Clients.S3, name); err != nil {
			return err
		}
	}
	return nil
}

func emptyAndDelete(ctx context.Context, c *s3.Client, name string) error {
	var token *string
	for {
		page, err := c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(name), ContinuationToken: token})
		if err != nil {
			if strings.Contains(err.Error(), "NoSuchBucket") {
				return nil
			}
			return fmt.Errorf("list %s: %w", name, err)
		}
		for _, obj := range page.Contents {
			_, err := c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(name), Key: obj.Key})
			if err != nil {
				return fmt.Errorf("delete object: %w", err)
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		token = page.NextContinuationToken
	}
	_, err := c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	return ignoreMissing(err)
}

func (rt *Runtime) sweepEC2(ctx context.Context, rows []sweep.Resource) error {
	c := rt.Clients.EC2
	inst, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err != nil {
		return fmt.Errorf("describe instances: %w", err)
	}
	var ids []string
	for _, res := range inst.Reservations {
		for _, in := range res.Instances {
			if tagged(in.Tags) || ledgerHas(rows, "instance", aws.ToString(in.InstanceId)) {
				ids = append(ids, aws.ToString(in.InstanceId))
			}
		}
	}
	for len(ids) > 0 {
		n := 20
		if len(ids) < n {
			n = len(ids)
		}
		_, err := c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: ids[:n]})
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("terminate: %w", err)
		}
		ids = ids[n:]
	}
	for _, row := range rows {
		if row.Kind != "volume" {
			continue
		}
		_, err := c.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(row.ID)})
		if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "IncorrectState") {
			return fmt.Errorf("delete volume %s: %w", row.ID, err)
		}
	}
	snaps, err := c.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}})
	if err != nil {
		return fmt.Errorf("describe snapshots: %w", err)
	}
	for _, s := range snaps.Snapshots {
		if strings.HasPrefix(aws.ToString(s.Description), "conf-") || ledgerHas(rows, "snapshot", aws.ToString(s.SnapshotId)) {
			_, err := c.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: s.SnapshotId})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete snapshot: %w", err)
			}
		}
	}
	addrs, err := c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if err != nil {
		return fmt.Errorf("describe addresses: %w", err)
	}
	for _, a := range addrs.Addresses {
		id := aws.ToString(a.AllocationId)
		if !ledgerHas(rows, "address", id) {
			continue
		}
		if a.AssociationId != nil {
			_, _ = c.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{AssociationId: a.AssociationId})
		}
		_, err := c.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(id)})
		if err != nil && !strings.Contains(err.Error(), "NotFound") {
			return fmt.Errorf("release address: %w", err)
		}
	}
	keys, err := c.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{})
	if err != nil {
		return fmt.Errorf("describe key pairs: %w", err)
	}
	for _, k := range keys.KeyPairs {
		if strings.HasPrefix(aws.ToString(k.KeyName), "conf-") {
			_, err := c.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: k.KeyName})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete key pair: %w", err)
			}
		}
	}
	groups, err := c.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{})
	if err != nil {
		return fmt.Errorf("describe security groups: %w", err)
	}
	for _, g := range groups.SecurityGroups {
		if strings.HasPrefix(aws.ToString(g.GroupName), "conf-") {
			_, err := c.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: g.GroupId})
			if err != nil && !strings.Contains(err.Error(), "NotFound") {
				return fmt.Errorf("delete security group: %w", err)
			}
		}
	}
	subs, err := c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{})
	if err != nil {
		return fmt.Errorf("describe subnets: %w", err)
	}
	for _, s := range subs.Subnets {
		if tagged(s.Tags) {
			_, err := c.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: s.SubnetId})
			if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "Dependency") {
				return fmt.Errorf("delete subnet: %w", err)
			}
		}
	}
	vpcs, err := c.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{})
	if err != nil {
		return fmt.Errorf("describe vpcs: %w", err)
	}
	for _, v := range vpcs.Vpcs {
		if tagged(v.Tags) {
			_, err := c.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: v.VpcId})
			if err != nil && !strings.Contains(err.Error(), "NotFound") && !strings.Contains(err.Error(), "Dependency") {
				return fmt.Errorf("delete vpc: %w", err)
			}
		}
	}
	return nil
}

func tagged(tags []ec2types.Tag) bool {
	for _, t := range tags {
		if aws.ToString(t.Key) == "vws:conformance-run" && strings.HasPrefix(aws.ToString(t.Value), "conf-") {
			return true
		}
		if aws.ToString(t.Key) == "Name" && strings.HasPrefix(aws.ToString(t.Value), "conf-") {
			return true
		}
	}
	return false
}

func (rt *Runtime) drySweep(ctx context.Context, rows []sweep.Resource) error {
	var left []string
	note := func(kind, id string) { left = append(left, kind+" "+id) }
	if rt.Clients != nil && rt.Clients.S3 != nil {
		out, err := rt.Clients.S3.ListBuckets(ctx, &s3.ListBucketsInput{})
		if err != nil {
			return fmt.Errorf("list buckets: %w", err)
		}
		for _, b := range out.Buckets {
			name := aws.ToString(b.Name)
			if strings.HasPrefix(name, "conf-") {
				note("bucket", name)
			}
		}
	}
	if rt.Clients != nil && rt.Clients.EC2 != nil {
		c := rt.Clients.EC2
		inst, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
		if err != nil {
			return fmt.Errorf("describe instances: %w", err)
		}
		for _, res := range inst.Reservations {
			for _, in := range res.Instances {
				if tagged(in.Tags) || ledgerHas(rows, "instance", aws.ToString(in.InstanceId)) {
					note("instance", aws.ToString(in.InstanceId))
				}
			}
		}
		vols, err := c.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{})
		if err != nil {
			return fmt.Errorf("describe volumes: %w", err)
		}
		for _, v := range vols.Volumes {
			if ledgerHas(rows, "volume", aws.ToString(v.VolumeId)) {
				note("volume", aws.ToString(v.VolumeId))
			}
		}
		snaps, err := c.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}})
		if err != nil {
			return fmt.Errorf("describe snapshots: %w", err)
		}
		for _, s := range snaps.Snapshots {
			if strings.HasPrefix(aws.ToString(s.Description), "conf-") || ledgerHas(rows, "snapshot", aws.ToString(s.SnapshotId)) {
				note("snapshot", aws.ToString(s.SnapshotId))
			}
		}
		keys, err := c.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{})
		if err != nil {
			return fmt.Errorf("describe key pairs: %w", err)
		}
		for _, k := range keys.KeyPairs {
			if strings.HasPrefix(aws.ToString(k.KeyName), "conf-") {
				note("key-pair", aws.ToString(k.KeyName))
			}
		}
	}
	for _, item := range left {
		fmt.Println("leftover", item)
	}
	if len(left) > 0 {
		return fmt.Errorf("%d leftover resources", len(left))
	}
	fmt.Println("leftovers 0")
	return nil
}

func ledgerHas(rows []sweep.Resource, kind, id string) bool {
	for _, r := range rows {
		if r.Kind == kind && r.ID == id && strings.HasPrefix(r.LedgerRunID, "conf-") {
			return true
		}
	}
	return false
}
