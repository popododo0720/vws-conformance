package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/popododo0720/vws-conformance/internal/sweep"
)

// errVerifiedAbsent means a paginated describe proved the id is gone and the
// ledger or name still belongs to a run this sweep may touch. The SDK call
// still runs so a negative case records that absence.
var errVerifiedAbsent = errors.New("verified absent")

func (rt *Runtime) proceed(err error) error {
	if err == nil || errors.Is(err, errVerifiedAbsent) {
		return nil
	}
	return err
}

func (rt *Runtime) noteFresh(name string) {
	rt.devMu.Lock()
	defer rt.devMu.Unlock()
	if rt.fresh == nil {
		rt.fresh = map[string]struct{}{}
	}
	rt.fresh[name] = struct{}{}
}

func (rt *Runtime) isFresh(name string) bool {
	rt.devMu.Lock()
	defer rt.devMu.Unlock()
	_, ok := rt.fresh[name]
	return ok
}

func (rt *Runtime) BucketFresh(name string) bool { return rt.isFresh(name) }

func (rt *Runtime) ours(rows []sweep.Resource, kind, id string) bool {
	owners := ledgerOwners(rows, kind, id, rt.ledgerScope())
	owners = mergeOwners(owners, runOwners(id))
	return AllowMutation(false, owners, nil, rt.keepRun)
}

func (rt *Runtime) permitLedger(rows []sweep.Resource, kind, id string, peers [][]string) error {
	owners := ledgerOwners(rows, kind, id, rt.ledgerScope())
	owners = mergeOwners(owners, runOwners(id))
	if !AllowMutation(false, owners, peers, rt.keepRun) {
		return fmt.Errorf("preserve %s %s", kind, id)
	}
	return nil
}

func (rt *Runtime) decideItem(kind, id string, item sweepItem, gone bool, obsErr error, names ...string) error {
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return fmt.Errorf("preserve %s %s: %w", kind, id, err)
	}
	if obsErr != nil && unsupportedAction(obsErr) {
		// DescribeInternetGateways is not served by every target. There is no
		// page to fail, so the ledger is the evidence and a conflict still preserves.
		return rt.permitLedger(rows, kind, id, item.peers)
	}
	if gone || (obsErr != nil && strings.Contains(obsErr.Error(), "NotFound")) {
		if rt.ours(rows, kind, id) {
			return errVerifiedAbsent
		}
		return fmt.Errorf("preserve %s %s", kind, id)
	}
	if obsErr != nil {
		return fmt.Errorf("preserve %s %s: %w", kind, id, obsErr)
	}
	owners := mergeOwners(item.owners, ledgerOwners(rows, kind, id, rt.ledgerScope()))
	for _, name := range names {
		owners = mergeOwners(owners, runOwners(name))
	}
	if kind == "volume" && rt.notedDevice(id) {
		owners = appendOwner(owners, rt.Cfg.RunID)
	}
	if !AllowMutation(false, owners, item.peers, rt.keepRun) {
		return fmt.Errorf("preserve %s %s", kind, id)
	}
	return nil
}

func (rt *Runtime) permitVolume(ctx context.Context, id string) error {
	item, gone, err := rt.observeVolume(ctx, sweepItem{id: id})
	return rt.decideItem("volume", id, item, gone, err)
}

func (rt *Runtime) permitAddress(ctx context.Context, id string) error {
	item, gone, err := rt.observeAddress(ctx, sweepItem{id: id})
	return rt.decideItem("address", id, item, gone, err)
}

func (rt *Runtime) permitInstance(ctx context.Context, id string) error {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return fmt.Errorf("preserve instance %s: ec2 client is unset", id)
	}
	matched, tags, err := rt.findInstance(ctx, id)
	item := sweepItem{owners: tagOwners(tags)}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve instance %s: ambiguous", id)
		}
		if err == nil {
			err = fmt.Errorf("preserve instance %s", id)
		}
		return rt.decideItem("instance", id, item, false, err)
	}
	return rt.decideItem("instance", id, item, false, nil)
}

func (rt *Runtime) permitTerminate(ctx context.Context, id string) error {
	peers, peerErr := rt.instanceAttachmentPeers(ctx, id)
	if peerErr != nil && !unsupportedAction(peerErr) && !strings.Contains(peerErr.Error(), "NotFound") {
		return fmt.Errorf("preserve instance %s: %w", id, peerErr)
	}
	if peerErr == nil && !AllowMutation(true, nil, peers, rt.keepRun) {
		return fmt.Errorf("preserve instance %s", id)
	}
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return fmt.Errorf("preserve instance %s: ec2 client is unset", id)
	}
	matched, tags, err := rt.findInstance(ctx, id)
	item := sweepItem{owners: tagOwners(tags), peers: peers}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve instance %s: ambiguous", id)
		}
		if err == nil {
			err = fmt.Errorf("preserve instance %s", id)
		}
		return rt.decideItem("instance", id, item, false, err)
	}
	return rt.decideItem("instance", id, item, false, nil)
}

func (rt *Runtime) permitTagged(ctx context.Context, id string) error {
	err := rt.permitInstance(ctx, id)
	if err == nil || errors.Is(err, errVerifiedAbsent) {
		return err
	}
	return rt.permitVolume(ctx, id)
}

func (rt *Runtime) permitKey(ctx context.Context, name string) error {
	matched, tags, err := rt.findKey(ctx, name)
	item := sweepItem{owners: tagOwners(tags)}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve key %s: ambiguous", name)
		}
		if err == nil {
			err = fmt.Errorf("preserve key %s", name)
		}
		return rt.decideItem("key-pair", name, item, false, err, name)
	}
	return rt.decideItem("key-pair", name, item, false, nil, name)
}

func (rt *Runtime) findKey(ctx context.Context, name string) (int, []ec2types.Tag, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return 0, nil, fmt.Errorf("ec2 client is unset")
	}
	out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeKeyPairsOutput, error) {
		return rt.Clients.EC2.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{KeyNames: []string{name}})
	})
	if err != nil || out == nil {
		return 0, nil, err
	}
	var matched int
	var tags []ec2types.Tag
	for _, key := range out.KeyPairs {
		if aws.ToString(key.KeyName) == name {
			matched++
			tags = key.Tags
		}
	}
	return matched, tags, nil
}

func (rt *Runtime) permitSnapshot(ctx context.Context, id string) error {
	matched, tags, desc, err := rt.findSnapshot(ctx, id)
	item := sweepItem{owners: tagOwners(tags)}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve snapshot %s: ambiguous", id)
		}
		if err == nil {
			err = fmt.Errorf("preserve snapshot %s", id)
		}
		return rt.decideItem("snapshot", id, item, false, err, desc)
	}
	return rt.decideItem("snapshot", id, item, false, nil, desc)
}

func (rt *Runtime) findSnapshot(ctx context.Context, id string) (int, []ec2types.Tag, string, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return 0, nil, "", fmt.Errorf("ec2 client is unset")
	}
	var matched int
	var tags []ec2types.Tag
	var desc string
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSnapshotsOutput, error) {
			return rt.Clients.EC2.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
				SnapshotIds: []string{id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe snapshots returned nothing")
		}
		for _, snap := range out.Snapshots {
			if aws.ToString(snap.SnapshotId) == id {
				matched++
				tags = snap.Tags
				desc = aws.ToString(snap.Description)
			}
		}
		return out.NextToken, nil
	})
	return matched, tags, desc, err
}

func (rt *Runtime) permitGroup(ctx context.Context, id string) error {
	matched, tags, name, err := rt.findGroup(ctx, id)
	item := sweepItem{owners: tagOwners(tags)}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve security-group %s: ambiguous", id)
		}
		if err == nil {
			err = fmt.Errorf("preserve security-group %s", id)
		}
		return rt.decideItem("security-group", id, item, false, err, name)
	}
	return rt.decideItem("security-group", id, item, false, nil, name)
}

func (rt *Runtime) findGroup(ctx context.Context, id string) (int, []ec2types.Tag, string, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return 0, nil, "", fmt.Errorf("ec2 client is unset")
	}
	var matched int
	var tags []ec2types.Tag
	var name string
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSecurityGroupsOutput, error) {
			return rt.Clients.EC2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
				GroupIds: []string{id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe security groups returned nothing")
		}
		for _, group := range out.SecurityGroups {
			if aws.ToString(group.GroupId) == id {
				matched++
				tags = group.Tags
				name = aws.ToString(group.GroupName)
			}
		}
		return out.NextToken, nil
	})
	return matched, tags, name, err
}

func (rt *Runtime) permitVpc(ctx context.Context, id string) error {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return fmt.Errorf("preserve vpc %s: ec2 client is unset", id)
	}
	matched, tags, err := rt.findVpc(ctx, id)
	return rt.decideCounted("vpc", id, matched, tags, err)
}

func (rt *Runtime) findVpc(ctx context.Context, id string) (int, []ec2types.Tag, error) {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return 0, nil, fmt.Errorf("ec2 client is unset")
	}
	var matched int
	var tags []ec2types.Tag
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeVpcsOutput, error) {
			return rt.Clients.EC2.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{id}, NextToken: token})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe vpcs returned nothing")
		}
		for _, vpc := range out.Vpcs {
			if aws.ToString(vpc.VpcId) == id {
				matched++
				tags = vpc.Tags
			}
		}
		return out.NextToken, nil
	})
	return matched, tags, err
}

func (rt *Runtime) permitSubnet(ctx context.Context, id string) error {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return fmt.Errorf("preserve subnet %s: ec2 client is unset", id)
	}
	var matched int
	var tags []ec2types.Tag
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeSubnetsOutput, error) {
			return rt.Clients.EC2.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
				SubnetIds: []string{id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe subnets returned nothing")
		}
		for _, sub := range out.Subnets {
			if aws.ToString(sub.SubnetId) == id {
				matched++
				tags = sub.Tags
			}
		}
		return out.NextToken, nil
	})
	return rt.decideCounted("subnet", id, matched, tags, err)
}

func (rt *Runtime) decideCounted(kind, id string, matched int, tags []ec2types.Tag, obsErr error) error {
	item := sweepItem{owners: tagOwners(tags)}
	if matched > 1 {
		return fmt.Errorf("preserve %s %s: ambiguous", kind, id)
	}
	if obsErr != nil || matched != 1 {
		if obsErr == nil {
			obsErr = fmt.Errorf("preserve %s %s", kind, id)
		}
		return rt.decideItem(kind, id, item, false, obsErr)
	}
	return rt.decideItem(kind, id, item, false, nil)
}

func (rt *Runtime) permitGateway(ctx context.Context, id string) error {
	if rt.Clients == nil || rt.Clients.EC2 == nil {
		return fmt.Errorf("preserve internet-gateway %s: ec2 client is unset", id)
	}
	var matched int
	var tags []ec2types.Tag
	var vpcs []string
	err := eachPage(func(token *string) (*string, error) {
		out, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DescribeInternetGatewaysOutput, error) {
			return rt.Clients.EC2.DescribeInternetGateways(ctx, &ec2.DescribeInternetGatewaysInput{
				InternetGatewayIds: []string{id}, NextToken: token,
			})
		})
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fmt.Errorf("describe internet gateways returned nothing")
		}
		for _, gw := range out.InternetGateways {
			if aws.ToString(gw.InternetGatewayId) != id {
				continue
			}
			matched++
			tags = gw.Tags
			for _, a := range gw.Attachments {
				vpcs = append(vpcs, aws.ToString(a.VpcId))
			}
		}
		return out.NextToken, nil
	})
	item := sweepItem{owners: tagOwners(tags), peers: rt.vpcPeers(ctx, vpcs)}
	if err != nil || matched != 1 {
		if matched > 1 {
			return fmt.Errorf("preserve internet-gateway %s: ambiguous", id)
		}
		if err == nil {
			err = fmt.Errorf("preserve internet-gateway %s", id)
		}
		return rt.decideItem("internet-gateway", id, item, false, err)
	}
	return rt.decideItem("internet-gateway", id, item, false, nil)
}

func (rt *Runtime) vpcPeers(ctx context.Context, vpcs []string) [][]string {
	if len(vpcs) == 0 {
		return nil
	}
	var peers [][]string
	for _, vpc := range vpcs {
		peers = append(peers, rt.describedVpcOwners(ctx, vpc))
	}
	return peers
}

func (rt *Runtime) describedVpcOwners(ctx context.Context, id string) []string {
	if id == "" || rt.Clients == nil || rt.Clients.EC2 == nil {
		return []string{}
	}
	matched, tags, err := rt.findVpc(ctx, id)
	if err != nil || matched != 1 {
		return []string{}
	}
	rows, err := readLedger(rt.Cfg.OutDir)
	if err != nil {
		return []string{}
	}
	return mergeOwners(tagOwners(tags), ledgerOwners(rows, "vpc", id, rt.ledgerScope()))
}

func (rt *Runtime) EC2Terminate(ctx context.Context, ids []string) error {
	var live []string
	for _, id := range ids {
		if rt.wasSwept(id) {
			continue
		}
		if err := rt.proceed(rt.permitTerminate(ctx, id)); err != nil {
			return err
		}
		live = append(live, id)
	}
	if len(live) == 0 {
		return nil
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.TerminateInstancesOutput, error) {
		return rt.Clients.EC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: live})
	})
	return err
}

func (rt *Runtime) EC2Stop(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := rt.proceed(rt.permitInstance(ctx, id)); err != nil {
			return err
		}
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.StopInstancesOutput, error) {
		return rt.Clients.EC2.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: ids})
	})
	return err
}

func (rt *Runtime) EC2Start(ctx context.Context, ids []string) error {
	for _, id := range ids {
		if err := rt.proceed(rt.permitInstance(ctx, id)); err != nil {
			return err
		}
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.StartInstancesOutput, error) {
		return rt.Clients.EC2.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: ids})
	})
	return err
}

func (rt *Runtime) EC2DeleteVolume(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitVolume(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteVolumeOutput, error) {
		return rt.Clients.EC2.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2DetachVolume(ctx context.Context, id, instanceID, device string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitVolume(ctx, id)); err != nil {
		return err
	}
	in := &ec2.DetachVolumeInput{VolumeId: aws.String(id)}
	if instanceID != "" {
		in.InstanceId = aws.String(instanceID)
	}
	if device != "" {
		in.Device = aws.String(device)
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DetachVolumeOutput, error) {
		return rt.Clients.EC2.DetachVolume(ctx, in)
	})
	return err
}

func (rt *Runtime) EC2AttachVolume(ctx context.Context, volumeID, instanceID, device string) (
	*ec2.AttachVolumeOutput, error,
) {
	if err := rt.proceed(rt.permitVolume(ctx, volumeID)); err != nil {
		return nil, err
	}
	if err := rt.proceed(rt.permitInstance(ctx, instanceID)); err != nil {
		return nil, err
	}
	return withDeadline(ctx, func(ctx context.Context) (*ec2.AttachVolumeOutput, error) {
		return rt.Clients.EC2.AttachVolume(ctx, &ec2.AttachVolumeInput{
			VolumeId: aws.String(volumeID), InstanceId: aws.String(instanceID), Device: aws.String(device),
		})
	})
}

func (rt *Runtime) EC2DeleteSnapshot(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitSnapshot(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSnapshotOutput, error) {
		return rt.Clients.EC2.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{SnapshotId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2ReleaseAddress(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitAddress(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.ReleaseAddressOutput, error) {
		return rt.Clients.EC2.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2DisassociateAddress(ctx context.Context, allocationID, associationID string) error {
	if err := rt.proceed(rt.permitAddress(ctx, allocationID)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DisassociateAddressOutput, error) {
		return rt.Clients.EC2.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{
			AssociationId: aws.String(associationID),
		})
	})
	return err
}

func (rt *Runtime) EC2AssociateAddress(ctx context.Context, allocationID, instanceID string) (
	*ec2.AssociateAddressOutput, error,
) {
	if err := rt.proceed(rt.permitAddress(ctx, allocationID)); err != nil {
		return nil, err
	}
	if instanceID != "" {
		if err := rt.proceed(rt.permitInstance(ctx, instanceID)); err != nil {
			return nil, err
		}
	}
	return withDeadline(ctx, func(ctx context.Context) (*ec2.AssociateAddressOutput, error) {
		return rt.Clients.EC2.AssociateAddress(ctx, &ec2.AssociateAddressInput{
			AllocationId: aws.String(allocationID), InstanceId: aws.String(instanceID),
		})
	})
}

func (rt *Runtime) EC2DeleteKeyPair(ctx context.Context, name string) error {
	if rt.wasSwept(name) {
		return nil
	}
	if err := rt.proceed(rt.permitKey(ctx, name)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteKeyPairOutput, error) {
		return rt.Clients.EC2.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: aws.String(name)})
	})
	return err
}

func (rt *Runtime) EC2DeleteSecurityGroup(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitGroup(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSecurityGroupOutput, error) {
		return rt.Clients.EC2.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2DeleteSubnet(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitSubnet(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteSubnetOutput, error) {
		return rt.Clients.EC2.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2DeleteVpc(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitVpc(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteVpcOutput, error) {
		return rt.Clients.EC2.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: aws.String(id)})
	})
	return err
}

func (rt *Runtime) EC2DetachInternetGateway(ctx context.Context, gatewayID, vpcID string) error {
	if err := rt.proceed(rt.permitGateway(ctx, gatewayID)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DetachInternetGatewayOutput, error) {
		return rt.Clients.EC2.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
			InternetGatewayId: aws.String(gatewayID), VpcId: aws.String(vpcID),
		})
	})
	return err
}

func (rt *Runtime) EC2DeleteInternetGateway(ctx context.Context, id string) error {
	if rt.wasSwept(id) {
		return nil
	}
	if err := rt.proceed(rt.permitGateway(ctx, id)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.DeleteInternetGatewayOutput, error) {
		return rt.Clients.EC2.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{
			InternetGatewayId: aws.String(id),
		})
	})
	return err
}

func (rt *Runtime) EC2CreateTags(ctx context.Context, ids []string, tags []ec2types.Tag) error {
	for _, id := range ids {
		if err := rt.proceed(rt.permitTagged(ctx, id)); err != nil {
			return err
		}
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*ec2.CreateTagsOutput, error) {
		return rt.Clients.EC2.CreateTags(ctx, &ec2.CreateTagsInput{Resources: ids, Tags: tags})
	})
	return err
}

func (rt *Runtime) bucketAbsent(ctx context.Context, name string) (bool, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return false, fmt.Errorf("s3 client is unset")
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*s3.HeadBucketOutput, error) {
		return rt.Clients.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(name)})
	})
	if err == nil {
		return false, nil
	}
	if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "NoSuch") ||
		strings.Contains(err.Error(), "404") {
		return true, nil
	}
	return false, err
}

func (rt *Runtime) permitBucket(ctx context.Context, name string) error {
	if rt.isFresh(name) {
		return nil
	}
	absent, err := rt.bucketAbsent(ctx, name)
	if err != nil {
		return fmt.Errorf("preserve bucket %s: %w", name, err)
	}
	rows, rowErr := readLedger(rt.Cfg.OutDir)
	if rowErr != nil {
		return fmt.Errorf("preserve bucket %s: %w", name, rowErr)
	}
	if absent {
		if rt.ours(rows, "bucket", name) {
			return errVerifiedAbsent
		}
		return fmt.Errorf("preserve bucket %s", name)
	}
	tags, tagErr := rt.bucketTags(ctx, name)
	if tagErr != nil {
		return fmt.Errorf("preserve bucket %s: %w", name, tagErr)
	}
	owners := mergeOwners(runOwners(name), tagOwners(tags))
	owners = mergeOwners(owners, ledgerOwners(rows, "bucket", name, rt.ledgerScope()))
	if !AllowMutation(false, owners, nil, rt.keepRun) {
		return fmt.Errorf("preserve bucket %s", name)
	}
	return nil
}

// S3CreateBucket always performs the create. Fresh means HeadBucket showed the
// name was absent before that success. us-east-1 also returns success when the
// bucket already existed, and that result is not fresh.
func (rt *Runtime) S3CreateBucket(ctx context.Context, in *s3.CreateBucketInput) (*s3.CreateBucketOutput, error) {
	if rt.Clients == nil || rt.Clients.S3 == nil {
		return nil, fmt.Errorf("s3 client is unset")
	}
	name := aws.ToString(in.Bucket)
	absent, absentErr := rt.bucketAbsent(ctx, name)
	out, err := withDeadline(ctx, func(ctx context.Context) (*s3.CreateBucketOutput, error) {
		return rt.Clients.S3.CreateBucket(ctx, in)
	})
	if absentErr == nil && absent && err == nil {
		rt.noteFresh(name)
	}
	return out, err
}

func (rt *Runtime) S3DeleteBucket(ctx context.Context, name string) error {
	if err := rt.proceed(rt.permitBucket(ctx, name)); err != nil {
		return err
	}
	_, err := withDeadline(ctx, func(ctx context.Context) (*s3.DeleteBucketOutput, error) {
		return rt.Clients.S3.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(name)})
	})
	return err
}

func (rt *Runtime) S3PutObject(ctx context.Context, in *s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	if err := rt.proceed(rt.permitBucket(ctx, aws.ToString(in.Bucket))); err != nil {
		return nil, err
	}
	return withDeadline(ctx, func(ctx context.Context) (*s3.PutObjectOutput, error) {
		return rt.Clients.S3.PutObject(ctx, in)
	})
}

func (rt *Runtime) S3DeleteObject(ctx context.Context, in *s3.DeleteObjectInput) (*s3.DeleteObjectOutput, error) {
	if err := rt.proceed(rt.permitBucket(ctx, aws.ToString(in.Bucket))); err != nil {
		return nil, err
	}
	return withDeadline(ctx, func(ctx context.Context) (*s3.DeleteObjectOutput, error) {
		return rt.Clients.S3.DeleteObject(ctx, in)
	})
}
