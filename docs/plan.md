# VWS AWS conformance suite: test plan and acceptance criteria

Status: design, 2026-10-15 KST. Author: vws-verify. Implementer: Cursor cloud agent. Runner: vws-verify on vws-test.
Goal: prove exact parity with the AWS APIs that VWS claims to implement (wire format, response shapes, error codes,
auth flows, SDK and CLI compatibility). Every failure becomes a tracked parity issue. Nothing is "known different" unless
an open issue says so.

## 1. What VWS implements today (research, read-only, vws-console-api main as of 2026-10-15)

All AWS-compatible traffic enters through **vws-console-api** (one process, three listeners). The backing services
(vws-compute, vws-storage, vws-network, vws-iam) speak gRPC only and are not AWS surfaces; they are tested only through
console-api.

| Surface | Listener (flag, default) | Protocol | Signing scope | API version |
| --- | --- | --- | --- | --- |
| EC2 Query (EC2, VPC, EBS, EIP, key pairs, SGs) | `-ec2-addr` 127.0.0.1:8081 | AWS Query, ec2 XML | `ec2`, region `vws-1` | 2016-11-15 |
| S3 REST, path-style only | `-s3-addr` 127.0.0.1:8082 | REST XML | `s3`, region `vws-1` | 2006-03-01 |
| IAM Query | `-iam-addr` 127.0.0.1:8084 | AWS Query | `iam`, region `vws-1` | 2010-05-08 |
| STS Query | same listener as IAM, chosen by credential scope | AWS Query | `sts`, region `vws-1` | 2011-06-15 |
| Ops (not AWS) | `-ops-addr` 127.0.0.1:8083 | /healthz /readyz /metrics | none | - |

Facts that drive the design:
- Region is `vws-1`; zones look like `vws-1a` (seen in vws-network#110 evidence). SigV4 clock skew allowance 5 min.
- Keys: root key from `VWS_ACCESS_KEY`/`VWS_SECRET_KEY` (Secret mount); with `-iam=grpc` all other keys are issued by
  vws-iam (`CreateAccessKey`) and verified via vws-iam `Authenticate` (secret never leaves vws-iam). STS `AssumeRole`
  returns temporary credentials (session token). Root key cannot `AssumeRole` (as in AWS).
- EC2 authorizes every action as `ec2:<Action>` on resource `*` (resource-level ARNs are a known follow-up).
  IAM/STS authorize as `iam:<Action>`/`sts:<Action>` except `GetCallerIdentity`.
- Unknown EC2 action -> Unimplemented error; unknown parameters are rejected (`rejectUnknown`), unsupported filters
  return `InvalidParameterValue`-style errors. The suite must record what code AWS would return instead.
- S3 rejects sub-resources it does not implement (`?tagging`, `?acl`, `?policy`, `?uploads`, `?uploadId`,
  `?versioning`, `?location`, `?cors`, `?lifecycle`, `?website`, `?delete`, `?versions`) explicitly.
- vws-test: k8s namespace `vws`, reached from the user's ubuntu-desktop (`~/vws-test`, kubectl). The Ceph RGW at
  10.77.0.101:8080 is the storage *backend*, not a VWS endpoint; never point the suite at it.

### 1.1 Operation inventory

"Impl" = dispatched by console-api today. Not-implemented operations that the suite still probes (to confirm the
error is the AWS-shaped one and to track the gap) are marked `gap`. Test IDs: `<SVC>-<OP>-<NNN>`; `NNN` 001-099 happy
path, 1xx documented errors, 2xx pagination/filters/tags/idempotency.

**EC2 / VPC / EBS (ec2 Query)**

| Service area | Operation | Impl | Test IDs |
| --- | --- | --- | --- |
| Instances | RunInstances | yes | EC2-RUN-001..003, 101..110, 201 (ClientToken), 202 (TagSpecifications) |
| Instances | DescribeInstances | yes | EC2-DI-001, 101..103, 201..206 (MaxResults/NextToken, Filter instance-state-name, instance-id, tag:*) |
| Instances | StartInstances / StopInstances / TerminateInstances | yes | EC2-START-*, EC2-STOP-*, EC2-TERM-* (state transitions, IncorrectInstanceState, InvalidInstanceID.NotFound/Malformed) |
| Instances | DescribeInstanceAttribute | yes | EC2-DIA-001..00n, 101 |
| Instances | RebootInstances, ModifyInstanceAttribute, DescribeInstanceStatus | gap | EC2-GAP-001..003 |
| Instance types | DescribeInstanceTypes | console only (gap on Query) | EC2-GAP-004 |
| Images | DescribeImages | yes | EC2-DIMG-001, 201..203 (Owners, ImageIds, IncludeDeprecated) |
| Images | CreateImage, DeregisterImage | console/gRPC only (gap) | EC2-GAP-005..006 |
| Key pairs | CreateKeyPair / ImportKeyPair / DeleteKeyPair / DescribeKeyPairs | yes | EC2-KP-* (KeyType rsa/ed25519, KeyFormat pem/ppk, IncludePublicKey, InvalidKeyPair.Duplicate, InvalidKeyPair.NotFound, idempotent delete) |
| VPC | CreateVpc / DeleteVpc / DescribeVpcs | yes | VPC-VPC-* (InvalidVpc.Range, InvalidVpcID.NotFound, DependencyViolation) |
| VPC | CreateSubnet / DeleteSubnet / DescribeSubnets | yes | VPC-SUB-* (AvailabilityZone vws-1a, InvalidSubnet.Range, InvalidSubnet.Conflict, InvalidSubnetID.NotFound) |
| VPC | CreateSecurityGroup / DeleteSecurityGroup / DescribeSecurityGroups / AuthorizeSecurityGroupIngress | yes | VPC-SG-* (InvalidGroup.Duplicate, InvalidGroup.NotFound, InvalidPermission.Duplicate) |
| VPC | RevokeSecurityGroupIngress, Authorize/RevokeSecurityGroupEgress, internet gateways, route tables, DescribeAvailabilityZones, DescribeRegions | gap | VPC-GAP-001..00n (DescribeAvailabilityZones/DescribeRegions are needed by the SDK/CLI ecosystem; flag high priority) |
| EIP | AllocateAddress / ReleaseAddress / AssociateAddress / DisassociateAddress / DescribeAddresses | yes | EC2-EIP-* (InvalidAllocationID.NotFound, InvalidAssociationID.NotFound, Resource.AlreadyAssociated) |
| EBS | CreateVolume / DeleteVolume / DescribeVolumes / AttachVolume / ModifyVolume | yes | EBS-VOL-* (InvalidVolume.NotFound, VolumeInUse, IncorrectState, InvalidParameterValue size) |
| EBS | DetachVolume, DescribeVolumesModifications | gap | EBS-GAP-001..002 |
| EBS | CreateSnapshot / DeleteSnapshot / DescribeSnapshots | yes | EBS-SNAP-* (InvalidSnapshot.NotFound, OwnerIds self) |
| Tags | CreateTags | yes | EC2-TAG-* (per resource type: i-, vol-, vpc-, subnet-, sg-, snap-, eipalloc-) |
| Tags | DeleteTags, DescribeTags | gap | EC2-GAP-0xx |

**S3 (REST, path-style)**

| Operation | Impl | Test IDs |
| --- | --- | --- |
| ListBuckets | yes | S3-LB-001 |
| CreateBucket / HeadBucket / DeleteBucket | yes | S3-BKT-* (BucketAlreadyOwnedByYou, InvalidBucketName, NoSuchBucket, BucketNotEmpty, LocationConstraint vws-1) |
| ListObjects (v1) / ListObjectsV2 | yes | S3-LO-* (prefix, delimiter/CommonPrefixes, max-keys, continuation-token, start-after, encoding-type=url, fetch-owner) |
| PutObject / GetObject / HeadObject / DeleteObject | yes | S3-OBJ-* (ETag, Content-MD5 BadDigest, x-amz-checksum-*, Range 206/InvalidRange, If-Match/If-None-Match 412/304, NoSuchKey, metadata x-amz-meta-*, idempotent delete 204) |
| Multipart, CopyObject, DeleteObjects, tagging, ACL, policy, versioning, presigned URL GET/PUT, virtual-host addressing | gap except presign (presign = SigV4 query auth; verify) | S3-GAP-* (assert the AWS error, e.g. NotImplemented 501, and file an issue each) |

**IAM (Query 2010-05-08)**

| Operation | Impl | Test IDs |
| --- | --- | --- |
| CreateUser / GetUser / ListUsers / DeleteUser | yes | IAM-USR-* (EntityAlreadyExists, NoSuchEntity, DeleteConflict, ValidationError name>64) |
| CreateLoginProfile / GetLoginProfile / UpdateLoginProfile / DeleteLoginProfile | yes | IAM-LP-* (PasswordPolicyViolation, NoSuchEntity) |
| CreateAccessKey / ListAccessKeys / DeleteAccessKey | yes | IAM-AK-* (LimitExceeded at 2 keys, new key usable for EC2/S3 within N s) |
| UpdateAccessKey (Inactive) | check, likely gap | IAM-AK-1xx |
| CreateGroup / GetGroup / ListGroups / DeleteGroup / AddUserToGroup / RemoveUserFromGroup / ListGroupsForUser | yes | IAM-GRP-* |
| CreateRole / GetRole / ListRoles / DeleteRole | yes | IAM-ROLE-* (MalformedPolicyDocument trust policy) |
| CreatePolicy / GetPolicy / GetPolicyVersion / ListPolicies / DeletePolicy | yes | IAM-POL-* (name 128 ok / 129 ValidationError per iam#66, Scope=Local, OnlyAttached) |
| CreatePolicyVersion / DeletePolicyVersion / ListPolicyVersions / SetDefaultPolicyVersion | vws-iam gRPC end to end; console-api Query gap (only GetPolicyVersion is exposed) | IAM-GAP-* (one gap issue per op) |
| Attach/Detach/ListAttached{User,Group,Role}Policies | yes | IAM-ATT-* |
| Put/Get/Delete/List{User,Group,Role}Policy (inline) | yes | IAM-INL-* |
| CreateVirtualMFADevice / EnableMFADevice / ListMFADevices / DeactivateMFADevice / DeleteVirtualMFADevice | vws-iam gRPC end to end; console-api Query gap | IAM-GAP-* (one per op) |
| Pagination (Marker/MaxItems/IsTruncated) on every List* | yes | IAM-*-2xx |

**STS (Query 2011-06-15)**

| Operation | Impl | Test IDs |
| --- | --- | --- |
| GetCallerIdentity | yes | STS-GCI-001 (root, IAM user, assumed-role ARN `arn:aws:sts::<acct>:assumed-role/<role>/<session>`) |
| AssumeRole | yes | STS-AR-* (DurationSeconds 900..MaxSessionDuration, ValidationError outside range, AccessDenied when trust policy denies, root key denied, temp creds sign EC2/S3/IAM, expired session token -> ExpiredToken) |
| GetSessionToken | vws-iam gRPC + sts service end to end; console-api STS Query gap (not dispatched) | STS-GAP-001 |
| AssumeRoleWithWebIdentity / SAML, GetAccessKeyInfo | gap | STS-GAP-* |


### 1.2 Verified 2026-10-15 17:4x KST (vws-iam main, vws-console-api main; read-only)

Policy versions, MFA and GetSessionToken are implemented end to end in **vws-iam** (gRPC handler -> store), but
**vws-console-api does not expose them on the IAM/STS Query surface**. They are parity gaps on the AWS surface, one
gap issue per operation (filed by dr eggbot).

| Feature | vws-iam (handler -> store) | console-api Query |
| --- | --- | --- |
| GetPolicyVersion | `internal/grpcapi/policies.go:270` | exposed, `internal/surface/iamquery/actions.go:377` |
| ListPolicyVersions | `policies.go:289` (max 5 versions) | missing |
| CreatePolicyVersion | `policies.go:312`, writes via `mutateOrValidate` on the policy store, 5-version quota, idempotent `request_id` replay | missing |
| DeletePolicyVersion | `policies.go:355` | missing |
| SetDefaultPolicyVersion | no dedicated RPC; done by `UpdatePolicy` with field mask `spec.default_version` (`policies.go:20`, `:198`) | missing |
| MFA: Get/List/CreateVirtual/Enable/Deactivate/Delete | `internal/grpcapi/mfa.go:54,64,86,121,139,157`; CreateVirtual persists with `store.Mutate` (`mfa.go:99`); MFA also checked in STS (`internal/sts/sts.go:275`) | missing (console sign-in uses MFA codes via vws-iam Login only) |
| GetSessionToken | `internal/grpcapi/token.go:173` -> `internal/sts/sts.go:441` | missing (STS Query dispatches only AssumeRole and GetCallerIdentity) |

Important: on **vws-test today console-api runs with the default `-iam=static`** (no `-iam` flag; see 5.1). In that
mode the IAM/STS listener serves only `GetCallerIdentity` for the root key. Every IAM case and AssumeRole will fail
or be unreachable until console-api is switched to `-iam=grpc`. That switch is a prerequisite for Phase 1 (open question 10).

The implementer must regenerate this inventory automatically (section 6, AC-2) rather than trust this table.

## 2. Tooling rules (non-negotiable)

1. **Go suite**: unmodified `github.com/aws/aws-sdk-go-v2` (service/ec2, s3, iam, sts) pinned in go.mod, latest
   release at implementation time; Dependabot/Renovate keeps it current. Only allowed overrides:
   `BaseEndpoint` per client (or `AWS_ENDPOINT_URL_EC2/_S3/_IAM/_STS`), `Region = "vws-1"`, `s3.Options.UsePathStyle = true`
   (path-style is the only addressing VWS supports; virtual-host is a tracked gap, see S3-GAP).
   No custom signer, no middleware that mutates requests, no `retry.AddWithErrorCodes` hacks. Middleware that only
   *reads* (to capture raw status, request id headers, and XML bodies for evidence) is allowed.
2. **CLI suite**: AWS CLI v2 (pinned version, official installer), bash scripts using `--endpoint-url` or
   `AWS_ENDPOINT_URL_<SVC>`, `--region vws-1`, `--output json`, asserting with `jq`. It proves CLI parsing and
   pagination (`--max-items`, `--starting-token`, `--no-paginate`) and waiters (`aws ec2 wait instance-running`).
3. **Raw-HTTP auth suite** (Go, `net/http` + SDK's own `aws/signer/v4` package unmodified) only for negative SigV4 cases
   the SDK cannot produce (tampered signature, stale date). Still no hand-written signing algorithm.
4. Credentials from env only: `VWS_CONF_ACCESS_KEY_ID`, `VWS_CONF_SECRET_ACCESS_KEY`, optional
   `VWS_CONF_SESSION_TOKEN`, plus `VWS_CONF_ACCOUNT_ID`. Endpoints: `VWS_CONF_EC2_ENDPOINT`, `VWS_CONF_S3_ENDPOINT`,
   `VWS_CONF_IAM_ENDPOINT` (IAM and STS share it). The bootstrap key is a vws-iam user `conformance-admin`
   created once by the operator, not the root key (root is used only for AUTH-ROOT cases if `VWS_CONF_ROOT_*` is set).

## 3. Test case design

Each case is a Go table entry (or one CLI script function) with this metadata, which also drives the report:

```go
type Case struct {
    ID       string   // "EC2-RUN-101"
    Service  string   // ec2|s3|iam|sts|auth
    Op       string   // "RunInstances"
    Kind     string   // happy|error|pagination|idempotency|tags|filter|auth
    DocURL   string   // https://docs.aws.amazon.com/... (required, lint fails without it)
    Expect   Expect   // HTTP status, AWS error code, required response fields
    Requires []string // fixtures: "vpc", "subnet", "image", "keypair"
    Slow     bool     // instance lifecycle etc.
}
```

### 3.1 Happy path (shape assertions)
- Assert every field the AWS API reference marks as always returned for that call, with type and format: IDs by
  prefix regex (`^i-[0-9a-f]{8,17}$`, `^vol-`, `^vpc-`, `^subnet-`, `^sg-`, `^snap-`, `^eipalloc-`, `^eipassoc-`,
  `^key-`), timestamps ISO 8601 UTC, enums from the SDK's generated enum types (`types.InstanceStateName` etc.),
  ARNs `arn:aws:iam::<12 digits>:user/<path><name>`, `requestId`/`x-amz-request-id` present.
- Assert values round-trip: what was created is what Describe/Get returns (CIDR, zone, size, tags, path).
- Assert raw XML namespace (`http://ec2.amazonaws.com/doc/2016-11-15/`, `https://iam.amazonaws.com/doc/2010-05-08/`,
  `https://sts.amazonaws.com/doc/2011-06-15/`, S3 `http://s3.amazonaws.com/doc/2006-03-01/`) via the read-only capture
  middleware.
- Doc URL pattern: `https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_<Op>.html`,
  `https://docs.aws.amazon.com/AmazonS3/latest/API/API_<Op>.html`,
  `https://docs.aws.amazon.com/IAM/latest/APIReference/API_<Op>.html`,
  `https://docs.aws.amazon.com/STS/latest/APIReference/API_<Op>.html`; error catalogs:
  `https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html`,
  `https://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html`,
  `https://docs.aws.amazon.com/IAM/latest/APIReference/CommonErrors.html`.

### 3.2 Documented errors
For each op, every error the AWS reference or error catalog documents that VWS can hit: assert HTTP status, `Code`
(via `smithy.APIError.ErrorCode()`), and that `Message` is non-empty. Minimum set:
- Common: `MissingParameter`, `InvalidParameterValue`, `InvalidParameterCombination`, `UnknownParameter` (EC2),
  `InvalidAction`, `UnauthorizedOperation` (EC2 deny), `AccessDenied` (IAM/STS/S3 deny), `DryRunOperation` /
  `UnauthorizedOperation` for `DryRun=true` (EC2), `RequestLimitExceeded`/`Throttling` (only if forced, skip otherwise).
- EC2 NotFound/Malformed variants per type: `InvalidInstanceID.NotFound`, `InvalidInstanceID.Malformed`,
  `InvalidVolume.NotFound`, `InvalidVpcID.NotFound`, `InvalidSubnetID.NotFound`, `InvalidGroup.NotFound`,
  `InvalidKeyPair.NotFound`, `InvalidAllocationID.NotFound`, `InvalidSnapshot.NotFound`, `InvalidAMIID.NotFound`.
- Idempotency: `IdempotentParameterMismatch` (same ClientToken, different params).
- IAM: `NoSuchEntity` (404), `EntityAlreadyExists` (409), `DeleteConflict` (409), `LimitExceeded`, `ValidationError`
  (400), `MalformedPolicyDocument`, `InvalidInput`.
- STS: `ValidationError`, `AccessDenied`, `ExpiredToken`, `InvalidClientTokenId`, `SignatureDoesNotMatch`.
- S3: `NoSuchBucket`, `NoSuchKey`, `BucketNotEmpty`, `BucketAlreadyOwnedByYou`, `InvalidBucketName`, `BadDigest`,
  `InvalidRange`, `PreconditionFailed`, `NotImplemented`, `AccessDenied`, `SignatureDoesNotMatch`,
  `RequestTimeTooSkewed`. For HEAD, assert status only (no body, per S3).

### 3.3 Pagination, idempotency, tags, filters
- Pagination: create N+1 resources under the run prefix, page with MaxResults/MaxItems/max-keys = small value, assert
  no duplicates, no loss, terminal page has no token, invalid token -> documented error. Use SDK paginators
  (`ec2.NewDescribeInstancesPaginator`, `iam.NewListUsersPaginator`, `s3.NewListObjectsV2Paginator`) to prove SDK
  compatibility, and CLI `--max-items` for CLI compatibility.
- Idempotency: RunInstances, CreateVolume, CreateSnapshot (where AWS supports ClientToken): same token + same params
  returns same resource ID; same token + different params -> `IdempotentParameterMismatch`.
- Tags: TagSpecifications on create, CreateTags on each taggable type, tag limits (50 tags, key 128 / value 256 chars,
  `aws:` prefix rejected), Describe filter `tag:<k>`, `tag-key`.
- Filters: for every Describe op, each filter name AWS documents that VWS claims; unsupported filter -> record the
  exact code returned; AWS returns `InvalidParameterValue` for unknown filter names on most calls.

### 3.4 Auth (AUTH-*)
| ID | Case | Expected (AWS) |
| --- | --- | --- |
| AUTH-001 | Valid SigV4 header auth, each surface | 200 |
| AUTH-002 | Signature byte flipped | 403 `SignatureDoesNotMatch` (S3, IAM, STS); EC2 `AuthFailure` |
| AUTH-003 | X-Amz-Date 6 min old/ahead | S3 `RequestTimeTooSkewed`; Query APIs `SignatureDoesNotMatch` / EC2 `AuthFailure` (record the exact message) |
| AUTH-004 | Credential scope region `us-east-1` instead of `vws-1` | `SignatureDoesNotMatch` / `AuthorizationHeaderMalformed` (S3) |
| AUTH-005 | Wrong service in scope (sign EC2 call as `s3`) | rejected |
| AUTH-006 | Unknown access key id | `InvalidClientTokenId` (IAM/STS), `InvalidAccessKeyId` (S3), `AuthFailure` (EC2) |
| AUTH-007 | No auth at all | `MissingAuthenticationToken` (Query), `AccessDenied` (S3) |
| AUTH-008 | Deleted / inactive access key | same as AUTH-006 within a documented propagation window |
| AUTH-009 | Presigned S3 GET/PUT (query auth), expired presign | 200 / 403 `AccessDenied` "Request has expired" |
| AUTH-010 | Unsigned payload, `STREAMING-AWS4-HMAC-SHA256-PAYLOAD` chunked upload (SDK default for some PutObject) | 200 or tracked gap |
| AUTH-011 | GetCallerIdentity as root, user, assumed role | ARNs and UserId formats per STS docs |
| AUTH-012 | AssumeRole -> temp creds call EC2, S3, IAM; tampered session token | 200; `InvalidClientTokenId` |
| AUTH-013 | Temp creds after expiry (DurationSeconds=900, clock wait or vws-iam test hook) | `ExpiredToken` (Slow) |
| AUTH-014 | Policy deny: user with explicit Deny on `ec2:RunInstances` | EC2 `UnauthorizedOperation` 403 |
| AUTH-015 | Policy deny: no allow (implicit) for `s3:PutObject`, `iam:CreateUser` | 403 `AccessDenied` |
| AUTH-016 | Trust policy does not allow caller to AssumeRole | 403 `AccessDenied` |
| AUTH-017 | Root key AssumeRole | `AccessDenied` (as AWS) |

## 4. Output

Per run: `out/<run-id>/report.json`, `out/<run-id>/report.md`, `out/<run-id>/raw/<case-id>.txt` (request line, status,
headers with `Authorization`, `X-Amz-Security-Token`, `X-Amz-Signature`, and any secret fields redacted, response body).

### 4.1 JSON schema (report.json)
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["schema_version","run_id","started_at","finished_at","target","tool_versions","summary","results"],
  "properties": {
    "schema_version": {"const": "1"},
    "run_id": {"type": "string", "pattern": "^conf-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}$"},
    "started_at": {"type": "string", "format": "date-time"},
    "finished_at": {"type": "string", "format": "date-time"},
    "target": {"type": "object", "required": ["name","region","console_api_version"],
      "properties": {"name": {"enum": ["vws-test","local-fake","local-grpc"]}, "region": {"type": "string"},
        "console_api_version": {"type": "string"}, "service_images": {"type": "object"}}},
    "tool_versions": {"type": "object", "required": ["go","aws_sdk_go_v2","aws_cli"]},
    "summary": {"type": "object", "required": ["pass","fail","skip","error","total"],
      "properties": {"pass":{"type":"integer"},"fail":{"type":"integer"},"skip":{"type":"integer"},
        "error":{"type":"integer"},"total":{"type":"integer"}}},
    "results": {"type": "array", "items": {"type": "object",
      "required": ["id","service","operation","kind","suite","status","doc_url","duration_ms"],
      "properties": {
        "id": {"type": "string"}, "service": {"enum": ["ec2","s3","iam","sts","auth"]},
        "operation": {"type": "string"},
        "kind": {"enum": ["happy","error","pagination","idempotency","tags","filter","auth","gap"]},
        "suite": {"enum": ["go-sdk","cli","raw-http"]},
        "status": {"enum": ["pass","fail","skip","error"]},
        "doc_url": {"type": "string", "pattern": "^https://docs\\.aws\\.amazon\\.com/"},
        "expected": {"type": "object"}, "actual": {"type": "object"},
        "reason": {"type": "string"}, "issue": {"type": "string"},
        "raw_ref": {"type": "string"}, "duration_ms": {"type": "integer"}}}}
  }
}
```
`fail` = VWS answered but differs from AWS. `error` = harness/infra problem (timeout, fixture failed); errors never
count as parity results and must be zero for a valid run. `skip` must carry a `reason` (e.g. "no image in vws-test").

### 4.2 Markdown (report.md)
Header (run id, target, versions, totals), then one table per service sorted by status (fail first):
`| ID | Operation | Kind | Suite | Status | Expected | Actual | Doc | Issue |`. A final "Gaps" table lists every `gap`
case. The same file is posted as the CI job summary.

### 4.3 Failure-to-issue template
`cmd/conf-issues` reads report.json and prints (dry-run by default; filing is done by a human or a bot with explicit
approval) one issue per failing case, deduplicated by `conformance:<case-id>` label/marker in the target repo
(console-api for wire/surface bugs; the owning service repo when the cause is in the backend):

```
Title: [parity] <Service> <Operation>: <one-line difference> (<case-id>)

Case: <case-id> (suite <go-sdk|cli|raw-http>)
AWS reference: <doc_url>
Run: <run-id> on <target>, console-api <version>, aws-sdk-go-v2 <ver>, aws-cli <ver>

Expected (AWS): HTTP <status>, code <Code>, fields <...>
Actual (VWS):   HTTP <status>, code <Code>, fields <...>

Redacted request/response: <raw excerpt>
Reproduce: go test ./ec2 -run 'TestConformance/<case-id>'   or   scripts/cli/ec2.sh <case-id>

Acceptance: case <case-id> passes on vws-test; no other case regresses.
Part of #<conformance tracking issue>
```
English only. No keys, tokens, or signatures in issues.

## 5. Operating on vws-test

- **Run id**: `conf-<UTC yyyymmddThhmmssZ>-<6 hex>`. Every created name or tag uses it: IAM names
  `conf-<id>-user1` (path `/conformance/`), buckets `conf-<id>-b1` (lowercase, <=63), EC2 resources tagged
  `vws:conformance=true` and `vws:conformance-run=<run-id>` plus Name `conf-<id>-...`.
- **Project/account**: run in a dedicated vws-test account/project `conformance` only; the suite refuses to start if
  `GetCallerIdentity` returns an account not in `VWS_CONF_ALLOWED_ACCOUNTS`, or if the endpoint host is not in
  `VWS_CONF_ALLOWED_HOSTS` (prevents pointing at anything but vws-test).
- **Idempotent setup**: shared fixtures (one VPC, one subnet in `vws-1a`, one SG, one key pair, image lookup) are
  get-or-create by run-id; a re-run with the same `VWS_CONF_RUN_ID` resumes.
- **Guaranteed teardown**: each test registers cleanup via `t.Cleanup` in reverse dependency order (instances ->
  EIP associations -> EIPs -> volumes -> snapshots -> SGs -> subnets -> VPCs -> objects -> buckets -> access keys ->
  policies attachments -> users/groups/roles/policies). A `TestMain` defer runs a run-scoped sweep even on panic or
  SIGINT/SIGTERM.
- **Sweeper**: `cmd/conf-sweep --older-than 6h [--run-id X] [--dry-run]` deletes only resources that carry the
  conformance tag / `conf-` prefix / `/conformance/` path in the allowed account. Run at CI start and end. Never
  touches untagged resources. Report leftovers as `error`.
- **Waits**: use SDK waiters where AWS has them (InstanceRunning, InstanceStopped, InstanceTerminated, VolumeAvailable,
  VolumeInUse, SnapshotCompleted, BucketExists) with a 10 min cap; a waiter that times out is `error`, not `fail`,
  unless the state reached is wrong.
- **Secrets**: never logged; the raw capture redacts `Authorization`, `X-Amz-Security-Token`, `X-Amz-Signature`,
  `X-Amz-Credential` (keep key id prefix 4 chars), and response fields `SecretAccessKey`, `SessionToken`,
  `KeyMaterial`, `Password`. A unit test feeds a known secret through every logger and fails if it appears.
  CLI scripts run with `set +x` around credential handling.
- **Concurrency**: one run at a time per account (CI `concurrency:` group); cases inside a run parallelize with
  `t.Parallel()` except instance-lifecycle and quota-bound cases.

### 5.1 Reaching vws-test from the box (verified read-only 2026-10-15 KST)

State observed (`kubectl-1.35 --kubeconfig kubeconfig-fip` in `~/vws-test` on the user's ubuntu-desktop, context `default`):
- `Deployment/console-api` in namespace `vws`, 1 replica, image `ghcr.io/popododo0720/vws/console-api:sha-a003d96`.
  Args: `-backend=grpc -console-addr=0.0.0.0:8080 -ec2-addr=0.0.0.0:8081 -s3-addr=0.0.0.0:8082 -compute-addr=compute:50051
  -network-addr=network:8080 -storage-addr=storage:7400 -console-origins=...`. **No `-iam`, `-iam-addr`, `-iam-endpoint`**,
  so IAM/STS uses `-iam=static` on the default `127.0.0.1:8084` (pod loopback only). Env: `VWS_ACCESS_KEY`,
  `VWS_SECRET_KEY` (root key), `VWS_CONSOLE_TOKEN`.
- `Service/console-api` is a LoadBalancer `10.77.0.241`, exposing **only 8080 (console JSON)**. Ports 8081/8082/8084
  are not in the container spec or the Service.
- vws-iam runs as `Deployment/vws-iam` (2 replicas, `127.0.0.1:5056/vws/iam:ddc2dac`, gRPC `:9095`, health `:9096`)
  in namespace `vws-iam-test`, with **no Service**, and console-api is not wired to it.

Procedure (run by vws-verify per test session, never left running):
1. Run everything from ubuntu-desktop `~/vws-test` with `K="./kubectl-1.35 --kubeconfig kubeconfig-fip"`; check
   `$K config current-context` is the vws-test cluster and the pod is in namespace `vws` before anything else.
2. Port-forward to the pod (port-forward enters the pod network namespace, so the loopback-bound 8084 works too):
   `$K -n vws port-forward deploy/console-api 18081:8081 18082:8082 18084:8084 >pf.log 2>&1 & PF=$!; trap 'kill $PF' EXIT`
3. The suite runs on ubuntu-desktop (or a self-hosted runner there) with
   `VWS_CONF_EC2_ENDPOINT=http://127.0.0.1:18081 VWS_CONF_S3_ENDPOINT=http://127.0.0.1:18082 VWS_CONF_IAM_ENDPOINT=http://127.0.0.1:18084`
   and `VWS_CONF_ALLOWED_HOSTS=127.0.0.1`.
   The box cannot reach the cluster directly; driving from the box means running the suite on ubuntu-desktop via the
   machine shell (box -> ubuntu-desktop -> port-forward). No forward is opened from the box.
4. The trap kills the forward when the run ends; check with `pgrep -f 'port-forward deploy/console-api'` = none.
Long term (CI): expose 8081/8082/8084 as Service ports (a vws-deploy chart change, through a PR) so the runner does not
need port-forward.

### 5.2 Minting the conformance key (prepared, NOT done; needs approval and `-iam=grpc`)

Prerequisite: console-api on vws-test runs `-iam=grpc -iam-endpoint <vws-iam Service>:9095 -iam-addr=0.0.0.0:8084`
(vws-deploy change plus a Service for vws-iam). Without it, non-root keys are not accepted on any surface.
1. Open the port-forward (5.1). Load the root key into the shell **only** from the cluster Secret, never echoing it:
   `export AWS_ACCESS_KEY_ID=$($K -n vws get secret <console-api secret> -o jsonpath='{.data.access-key}' | base64 -d)` (same for
   the secret key), with `set +x`, `HISTCONTROL=ignorespace`, and no `tee`.
2. With the AWS CLI against `http://127.0.0.1:18084`, `--region vws-1`:
   `aws iam create-user --user-name conformance-admin --path /conformance/ --tags Key=vws:conformance,Value=true`;
   `aws iam create-policy --policy-name conformance-admin --path /conformance/ --policy-document file://conformance-admin.json`
   (allow `ec2:*`, `s3:*`, `iam:*`, `sts:*`; the suite narrows per test with its own users); `aws iam attach-user-policy`.
   Create a role `conformance-assume` whose trust policy allows `conformance-admin` (for STS cases).
3. `aws iam create-access-key --user-name conformance-admin --output json > "$(mktemp -p ~/.config/vws-conformance)"`
   with `umask 077`; never print it. Convert to an env file `~/.config/vws-conformance/vws-test.env` (mode 0600,
   outside any git repo) holding `VWS_CONF_ACCESS_KEY_ID`, `VWS_CONF_SECRET_ACCESS_KEY`, `VWS_CONF_ACCOUNT_ID`; delete
   the JSON. The suite reads env only: `set -a; . ~/.config/vws-conformance/vws-test.env; set +a`.
4. For CI, the user (not a bot) stores the same values as GitHub environment `vws-test` secrets in vws-conformance.
5. Hygiene: `.gitignore` covers `*.env`, `out/`; a pre-commit/CI secret scan (gitleaks) runs on the repo; the redaction
   test (AC-8) uses the real key length pattern. Rotate the key by creating a second key, updating the env file and
   secret, then deleting the old one; delete the user when the program ends.
6. Unset `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` (root) right after step 3; the suite never uses root except AUTH-017.

## 6. Repo and layout

Recommendation: **new public repo `popododo0720/vws-conformance`.**
Rationale: it tests the product as a black box from the outside, so it must not import any VWS module (keeps it
honest: only the AWS SDK/CLI talk to VWS); it has its own dependency cadence (SDK/CLI upgrades should not churn
vws-deploy); results and issues span every service repo; vws-deploy's gate rules (no Closes, R2 scopes) are tuned for
deploy code. It still adopts the shared guard files (AGENTS.md, QUALITY.md, PR template, verify-gate, english check)
copied from vws-deploy like the other repos. Alternative (dir `conformance/` in vws-deploy) is acceptable only if
the user prefers fewer repos; downside is coupling and gate friction.

```
vws-conformance/
  AGENTS.md  QUALITY.md  README.md  LICENSE  .github/ (PR template, verify-gate, english, conformance.yml)
  go.mod                     # only aws-sdk-go-v2 + stdlib + testify-free; no vws-* imports (CI checks)
  internal/harness/          # env config, allow-lists, run id, clients (endpoint override only), capture middleware, redaction
  internal/report/           # result collection, JSON (schema in schema/report.schema.json) + markdown writers
  internal/fixtures/         # get-or-create VPC/subnet/SG/keypair/image, cleanup registry
  internal/expect/           # shape assertions (ID regexes, enums, ARN, time), error assertions
  ec2/  vpc/  ebs/  s3/  iam/  sts/  auth/   # *_test.go, one file per operation, table of Case
  inventory/                 # inventory.yaml (op -> impl/gap -> case IDs) + test that every Impl op has >=1 happy and >=1 error case
  scripts/cli/               # ec2.sh s3.sh iam.sh sts.sh lib.sh (jq assertions, emit JSON lines merged into report)
  cmd/conf-report/  cmd/conf-sweep/  cmd/conf-issues/  cmd/conf-inventory/
  schema/report.schema.json
  Makefile                   # make test-local | test-vws | sweep | report
```
`cmd/conf-inventory` regenerates the implemented-operation list from a vws-console-api checkout (grep of the dispatch
switch in `internal/surface/ec2query`, the action map in `internal/surface/iamquery/actions.go`, the mux in
`internal/surface/s3rest`) and fails CI when an implemented op has no case or a case targets an op no longer present.

## 7. CI

- `conformance-local.yml` (on PR to vws-conformance, and callable from vws-console-api PRs): starts
  vws-console-api `-backend=fake -iam=fake` (as `e2e/run.sh` does) on GitHub-hosted runners, runs the full suite
  with `target=local-fake`. Cases needing real VMs/storage are `skip` there. Required check for vws-conformance.
  Optional `local-grpc` mode using vws-console-api `e2e/stack.sh` when service binaries are available.
- `conformance-vws-test.yml`: `schedule` nightly 03:00 KST (18:00 UTC) + `workflow_dispatch` (inputs: services,
  run_id, keep_resources=false). Runs on a **self-hosted runner** labelled `vws-test` on a host that can reach the
  vws-test console-api endpoints (user approval needed to register it). Steps: sweep -> go test -json ->
  CLI scripts -> merge report -> sweep -> upload `out/` artifact -> write job summary. Credentials from GitHub
  environment `vws-test` secrets (approval-protected). Never runs on fork PRs.
- Nightly regression rule: a case that passed in the previous nightly and fails now marks the job red.
  Fails already linked to an open issue do not (they are listed as "known, tracked").

## 8. Acceptance criteria for the implementation

- AC-1: Repo builds with `go vet` and golangci-lint clean; go.mod imports no `github.com/popododo0720/vws-*` module
  and no fork/replace of aws-sdk-go-v2 (CI check).
- AC-2: `inventory/inventory.yaml` lists every operation dispatched by vws-console-api main at implementation time
  (section 1.1 at minimum: 35 EC2 actions (25 mutating + 10 Describe), 9 S3 routes, all IAM actions in actions.go including Attach/Detach/
  ListAttached/Put/Get/Delete/List x User/Group/Role, STS AssumeRole and GetCallerIdentity); an inventory test proves
  each implemented op has >= 1 happy and >= 1 documented-error case, and each listed gap has a probe case.
- AC-3: Every case has a `docs.aws.amazon.com` URL; a lint test fails otherwise. Spot-check: 10 random URLs return 200.
- AC-4: Every AUTH-001..017 case exists (skip allowed only with reason, e.g. AUTH-013 behind `-slow`).
- AC-5: Both suites (go-sdk and cli) cover at least one happy and one error case per service; CLI covers paginated
  list and one waiter.
- AC-6: `report.json` validates against `schema/report.schema.json` (test included); `report.md` renders the tables;
  `conf-issues` prints the template for each fail, dry-run by default.
- AC-7: Two consecutive full runs against vws-test with the same credentials both complete with `error=0`, and
  `conf-sweep --dry-run` afterwards finds 0 leftovers. Killing a run mid-way (SIGTERM) and running the sweeper leaves 0
  leftovers.
- AC-8: Secret-leak test passes; grep of `out/` for the configured secret key and session token finds nothing.
- AC-9: Allow-list guard: pointing at a non-allowed host/account exits non-zero before any mutating call (test).
- AC-10: local-fake CI job green on the PR; vws-test workflow runs once via workflow_dispatch (run by vws-verify).
- AC-11: README documents env vars, how to add a case, and how to triage a failure into an issue. English only.

Definition of done for the program (not the PR): every `fail` in the first vws-test run has an open parity issue, and
the tracking issue lists them.

## 9. Phased rollout (depth first)

1. **Phase 0, harness**: config, allow-lists, run id, capture+redaction, report JSON/MD, sweeper, inventory tool,
   local-fake CI. Exit: AC-1, AC-6, AC-8, AC-9.
2. **Phase 1, IAM + STS end to end** (no VMs needed, exercises auth everything else depends on): all IAM/STS ops,
   AUTH-001..017, CLI iam.sh/sts.sh. Exit: first vws-test run, issues filed for fails.
3. **Phase 2, S3** (objects, listings, presign, conditional requests).
4. **Phase 3, VPC + EC2 key pairs/SGs/EIPs** (cheap resources).
5. **Phase 4, EBS volumes/snapshots, then instances** (slow, quota-bound; RunInstances lifecycle, ClientToken,
   attach/modify).
6. **Phase 5, gaps and nightly**: probe cases for all gap ops, nightly schedule, regression rule.
Each phase is one PR (Part of the tracking issue), judged by vws-verify with a vws-test run as evidence.

## 10. Open questions

1. vws-test endpoints: what host/ports expose console-api's EC2 (8081), S3 (8082) and IAM/STS (8084) listeners from
   outside the cluster (Service type, ingress, TLS)? Is console-api on vws-test running `-iam=grpc`?
2. Self-hosted runner: may we register a GitHub Actions runner on a host that reaches vws-test (ubuntu-desktop or a
   pod in vws-test)? Needs the user's approval.
3. Bootstrap credentials: who creates the `conformance-admin` IAM user/key in vws-test and stores it as a GitHub
   environment secret? (vws-verify must not handle key values beyond env injection.)
4. Repo choice: new `popododo0720/vws-conformance` (recommended) or `vws-deploy/conformance/`?
5. Account id / ARN partition: VWS emits `arn:aws:`; should the suite require `aws` partition exactly, or accept a
   VWS partition? (Parity says `aws`.)
6. Gap policy: should every probed-but-unimplemented op (e.g. DescribeRegions, DescribeAvailabilityZones,
   DetachVolume, RevokeSecurityGroupIngress, multipart upload) get its own parity issue on the first run, or one
   umbrella issue per service?
7. Expected-code ambiguity: where AWS docs are vague (e.g. EC2 skew -> `AuthFailure` vs `RequestExpired`), may we
   record real-AWS responses once from a sandbox AWS account as golden files? That would need an AWS account.
8. Instance cases need a bootable image and quota on vws-test; which image and how many concurrent instances are OK?
9. Resolved: gap/failure issues are filed by dr eggbot, one per operation. Repo resolved: popododo0720/vws-conformance (#1 = harness + IAM + STS). ARNs must match the AWS format, `aws` partition.
10. Blocking for Phase 1: who approves and lands switching vws-test console-api to `-iam=grpc` (plus a Service for vws-iam in `vws-iam-test`, and container/Service ports 8081/8082/8084)? Until then only GetCallerIdentity (root) is reachable on IAM/STS.


## 11. Owner decisions (2026-10-15)

These decisions override earlier notes in this plan where they conflict.

1. No self-hosted runner and no nightly job. CI runs only the pull-request job against console-api's fake backend. The vws-test run is manual. See README for the command. `conformance-vws-test.yml` is not added.
2. The suite never calls real AWS. There is no sandbox account, no real-AWS mode, and no real-AWS endpoint. Expected behavior comes from the official AWS docs only. The endpoint allow-list rejects AWS endpoint names, including `amazonaws.com` and any host ending in `.amazonaws.com`, even if that host is also listed in `VWS_CONF_ALLOWED_HOSTS`. The HTTP client repeats the check at connect time.
3. Issue #1 scope for this repository: harness, then S3, then EC2/VPC (including the EBS operations on the EC2 listener). IAM and STS cases are written and skipped with the reason `vws-deploy#310` until that change lands.

## 12. Additional acceptance criteria

- AC-12: Every run has a unique run id. Every resource name and tag the suite creates starts with that run's prefix.
- AC-13: Cleanup runs on success, failure, and panic. `TestMain` defers teardown. A signal hook runs the same teardown on SIGINT and SIGTERM.
- AC-14: A pre-run sweep deletes leftover resources from earlier runs that carry the suite prefix (`conf-` on the name, the `vws:conformance-run` tag, the snapshot description, or the `/conformance/` path). It does not delete anything else.
- AC-15: Cases are grouped by service and operation group. `cmd/conf-run --service s3 --group buckets` runs only that group.
- AC-16: Instance launches use `t2.nano` (the smallest type the fake backend accepts). Volumes use 1 GiB. Default concurrency is 1 (`VWS_CONF_PARALLEL`, maximum used by the suite default is 2 only when set).
- AC-17: The full suite runs only with `--full`, `VWS_CONF_FULL=1`, or a `workflow_dispatch` run. A pull request runs the unit tests plus the groups whose files changed. A change under the harness, report, schema, module files, or CI selector is a big change and runs the full suite. There is no `schedule` trigger.
