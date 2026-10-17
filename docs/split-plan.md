# Read-only split

This file lists the cases removed from the read-only branch and the pull requests that put them back.

## Sequence

1. This branch holds the harness and the cases that create nothing. The lead opens the pull request.
2. A standalone cleanup ownership-gate pull request lands next. A test that creates a resource must delete that resource even when the test fails. No creating case is merged before that gate. Manual cleanup of leftovers is not a substitute for the gate.
3. Creating cases come back one service at a time, in small pull requests on top of the gate. The order is EC2, then EBS, then VPC, then S3, then IAM and STS. The lead files the tracking issues from this list.

`TestProductionCodeHasNoMutatingSDKMethod` fails the build when production code selects a mutating method on `*ec2.Client`, `*s3.Client`, `*iam.Client`, or `*sts.Client`. A method value counts. The gate pull request replaces that test with a rule that allows those calls only inside the gate.

## Cases that stay

These cases call `Describe`, `List`, `Get`, or `Head`, or they send a read-only HTTP request. They do not create a resource.

EC2 keeps `EC2-DI-101`, `EC2-DI-102`, `EC2-DIA-101`, `EC2-DIMG-001`, `EC2-DIMG-101`, `EC2-KP-103`, `EC2-EIP-103`, `EC2-GAP-003`, `EC2-GAP-004`, and `EC2-GAP-008`.

EBS keeps `EBS-VOL-104`, `EBS-SNAP-102`, and `EBS-GAP-002`.

VPC keeps `VPC-VPC-104`, `VPC-SUB-103`, `VPC-SG-104`, `VPC-GAP-004`, `VPC-GAP-005`, and `VPC-GAP-007`.

S3 keeps `S3-LB-001`, `S3-LB-101`, `S3-BKT-102`, `S3-LO-101`, and `S3-LO-102`.

IAM and STS keep `IAM-USR-003`, `IAM-USR-102`, `IAM-GRP-101`, `IAM-GAP-002`, `STS-GCI-001`, and `STS-GCI-101`.

Auth keeps `AUTH-001`, `AUTH-002`, `AUTH-003`, `AUTH-004`, `AUTH-005`, `AUTH-006`, `AUTH-007`, `AUTH-008`, `AUTH-011`, and `AUTH-013`.

`EC2-EIP-103`, `EBS-VOL-104`, `EBS-SNAP-102`, `VPC-VPC-104`, `VPC-SUB-103`, and `VPC-SG-104` used to allocate a resource and then describe a deleted id. They now describe a fixed missing id and create nothing.

## EC2

- `EC2-RUN-001` RunInstances
- `EC2-RUN-101` RunInstances
- `EC2-RUN-110` RunInstances
- `EC2-RUN-201` RunInstances
- `EC2-RUN-202` RunInstances
- `EC2-DI-001` DescribeInstances, after RunInstances
- `EC2-DI-201` DescribeInstances, after RunInstances
- `EC2-DIA-001` DescribeInstanceAttribute, after RunInstances
- `EC2-STOP-001` StopInstances
- `EC2-STOP-101` StopInstances
- `EC2-START-001` StartInstances
- `EC2-START-101` StartInstances
- `EC2-TERM-001` TerminateInstances
- `EC2-TERM-101` TerminateInstances
- `EC2-KP-001` CreateKeyPair
- `EC2-KP-002` DescribeKeyPairs, after CreateKeyPair
- `EC2-KP-003` DeleteKeyPair
- `EC2-KP-004` ImportKeyPair
- `EC2-KP-101` CreateKeyPair
- `EC2-KP-102` DeleteKeyPair
- `EC2-KP-104` ImportKeyPair
- `EC2-EIP-001` AllocateAddress
- `EC2-EIP-002` AssociateAddress
- `EC2-EIP-003` DescribeAddresses, after AllocateAddress
- `EC2-EIP-004` DisassociateAddress
- `EC2-EIP-005` ReleaseAddress
- `EC2-EIP-101` ReleaseAddress
- `EC2-EIP-102` DisassociateAddress
- `EC2-EIP-104` AssociateAddress
- `EC2-EIP-105` AllocateAddress
- `EC2-TAG-001` CreateTags
- `EC2-TAG-101` CreateTags
- `EC2-GAP-001` RebootInstances
- `EC2-GAP-002` ModifyInstanceAttribute
- `EC2-GAP-005` CreateImage
- `EC2-GAP-006` DeregisterImage
- `EC2-GAP-007` DeleteTags

## EBS

- `EBS-VOL-001` CreateVolume
- `EBS-VOL-002` DescribeVolumes, after CreateVolume
- `EBS-VOL-003` AttachVolume
- `EBS-VOL-004` ModifyVolume
- `EBS-VOL-005` DeleteVolume
- `EBS-VOL-006` DetachVolume
- `EBS-VOL-101` DeleteVolume
- `EBS-VOL-102` DeleteVolume
- `EBS-VOL-103` CreateVolume
- `EBS-VOL-105` AttachVolume
- `EBS-VOL-106` ModifyVolume
- `EBS-VOL-107` DetachVolume
- `EBS-SNAP-001` CreateSnapshot
- `EBS-SNAP-002` DescribeSnapshots, after CreateSnapshot
- `EBS-SNAP-003` DeleteSnapshot
- `EBS-SNAP-101` DeleteSnapshot
- `EBS-SNAP-103` CreateSnapshot

## VPC

- `VPC-VPC-001` CreateVpc
- `VPC-VPC-002` DescribeVpcs, after CreateVpc
- `VPC-VPC-003` DeleteVpc
- `VPC-VPC-101` CreateVpc
- `VPC-VPC-102` DeleteVpc
- `VPC-VPC-103` DeleteVpc
- `VPC-SUB-001` CreateSubnet
- `VPC-SUB-002` DescribeSubnets, after CreateSubnet
- `VPC-SUB-003` DeleteSubnet
- `VPC-SUB-101` CreateSubnet
- `VPC-SUB-102` DeleteSubnet
- `VPC-SG-001` CreateSecurityGroup
- `VPC-SG-002` AuthorizeSecurityGroupIngress
- `VPC-SG-003` DescribeSecurityGroups, after CreateSecurityGroup
- `VPC-SG-004` DeleteSecurityGroup
- `VPC-SG-101` CreateSecurityGroup
- `VPC-SG-102` DeleteSecurityGroup
- `VPC-SG-103` AuthorizeSecurityGroupIngress
- `VPC-GAP-001` RevokeSecurityGroupIngress
- `VPC-GAP-002` AuthorizeSecurityGroupEgress
- `VPC-GAP-003` RevokeSecurityGroupEgress
- `VPC-GAP-006` CreateInternetGateway

## S3

- `S3-BKT-001` CreateBucket
- `S3-BKT-002` HeadBucket, after CreateBucket
- `S3-BKT-003` DeleteBucket
- `S3-BKT-101` CreateBucket
- `S3-BKT-103` DeleteBucket
- `S3-BKT-104` CreateBucket
- `S3-BKT-105` DeleteBucket
- `S3-OBJ-001` PutObject
- `S3-OBJ-002` GetObject, after CreateBucket
- `S3-OBJ-003` HeadObject, after CreateBucket
- `S3-OBJ-004` DeleteObject
- `S3-OBJ-005` GetObject, after CreateBucket
- `S3-OBJ-101` GetObject, after CreateBucket
- `S3-OBJ-102` PutObject
- `S3-OBJ-103` GetObject, after CreateBucket
- `S3-OBJ-104` GetObject, after CreateBucket
- `S3-OBJ-105` DeleteObject
- `S3-OBJ-106` HeadObject, after CreateBucket
- `S3-OBJ-107` DeleteObject
- `S3-LO-001` ListObjects, after CreateBucket
- `S3-LO-002` ListObjectsV2, after CreateBucket
- `S3-LO-003` ListObjectsV2, after CreateBucket
- `S3-GAP-001` CopyObject
- `S3-GAP-002` DeleteObjects
- `S3-GAP-003` PutObjectTagging
- `S3-GAP-004` GetObjectAcl, after CreateBucket
- `S3-GAP-005` PutBucketPolicy
- `S3-GAP-006` CreateMultipartUpload
- `S3-GAP-007` PutBucketVersioning
- `S3-GAP-008` GetBucketLocation, after CreateBucket
- `S3-GAP-009` PutBucketCors
- `S3-GAP-010` PutBucketLifecycleConfiguration
- `S3-GAP-011` PutBucketWebsite
- `S3-GAP-012` ListObjectVersions, after CreateBucket
- `S3-GAP-014` GetObject, after CreateBucket
- `S3-GAP-015` HeadBucket, after CreateBucket
- `S3-GAP-016` PutBucketLifecycleConfiguration

## IAM and STS

- `IAM-USR-001` CreateUser
- `IAM-USR-002` GetUser, after CreateUser
- `IAM-USR-004` DeleteUser
- `IAM-USR-101` CreateUser
- `IAM-USR-103` DeleteUser
- `IAM-USR-104` CreateUser
- `IAM-AK-001` CreateAccessKey
- `IAM-AK-101` CreateAccessKey
- `IAM-GRP-001` CreateGroup
- `IAM-ROLE-001` CreateRole
- `IAM-ROLE-101` CreateRole
- `IAM-POL-001` CreatePolicy
- `IAM-POL-101` CreatePolicy
- `IAM-GAP-001` CreatePolicyVersion
- `IAM-GAP-003` DeletePolicyVersion
- `IAM-GAP-004` SetDefaultPolicyVersion
- `IAM-GAP-005` CreateVirtualMFADevice
- `IAM-GAP-006` UpdateAccessKey
- `STS-AR-001` AssumeRole
- `STS-AR-101` AssumeRole
- `STS-GAP-001` GetSessionToken
- `STS-GAP-002` AssumeRoleWithWebIdentity
- `AUTH-009` GetObject, after CreateBucket
- `AUTH-010` PutObject
- `AUTH-012` AssumeRole
- `AUTH-014` RunInstances
- `AUTH-015` PutObject
- `AUTH-016` AssumeRole
- `AUTH-017` AssumeRole

`fillIAM` also emitted a happy id `IAM-H-<operation>` and an error id `IAM-E-<operation>` for every implemented IAM operation that had no explicit case. The mutating placeholders below were removed. The `Get` and `List` placeholders were skip stubs with no runner. They were not copied. They return in the IAM pull request.

The mutating `IAM-H-` placeholders cover `CreateLoginProfile`, `UpdateLoginProfile`, `DeleteLoginProfile`, `DeleteAccessKey`, `DeleteGroup`, `AddUserToGroup`, `RemoveUserFromGroup`, `DeleteRole`, `DeletePolicy`, `AttachUserPolicy`, `DetachUserPolicy`, `PutUserPolicy`, `DeleteUserPolicy`, `AttachGroupPolicy`, `DetachGroupPolicy`, `PutGroupPolicy`, `DeleteGroupPolicy`, `AttachRolePolicy`, `DetachRolePolicy`, `PutRolePolicy`, and `DeleteRolePolicy`.

The mutating `IAM-E-` placeholders cover the same operations.

The `Get` and `List` placeholders were not copied. They are `GetLoginProfile`, `ListAccessKeys`, `GetGroup`, `ListGroups`, `ListGroupsForUser`, `GetRole`, `ListRoles`, `GetPolicy`, `GetPolicyVersion`, `ListPolicies`, `ListAttachedUserPolicies`, `GetUserPolicy`, `ListUserPolicies`, `ListAttachedGroupPolicies`, `GetGroupPolicy`, `ListGroupPolicies`, `ListAttachedRolePolicies`, `GetRolePolicy`, and `ListRolePolicies`. `IAM-E-ListUsers` is the error placeholder for `ListUsers`. An `IAM-H-` id exists for each `Get` and `List` operation in that set. An `IAM-E-` id exists for each of them except `ListUsers`.
