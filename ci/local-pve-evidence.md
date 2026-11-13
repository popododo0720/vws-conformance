# Local Proxmox job evidence

## Issue #139: instance lifecycle restoration, first part

Run on November 13, 2026. `bash scripts/local-pve.sh` exits **0**, using
console-api `05840905e958f6ad5e57e200931d542798b22d73` and IAM
`d01a814a91afba7d884d9f5b0bf4ad7a23110a91`, unchanged from CI.
The additional EC2 instances invocation runs all 14 cases: 1 passes and 13
have linked known failures, with no skips, harness errors or untracked failures.

This first independently mergeable part restores 11 cases:
`EC2-RUN-001/101/110/201/202`, `EC2-DI-001/201`, `EC2-DIA-001`,
`EC2-STOP-001`, `EC2-START-001`, and `EC2-TERM-001`.
The existing `engine.Groups()` registration and `conf-select` instances
prefix already select them. Key pairs, `EC2-STOP/START/TERM-101`, and
`EC2-GAP-001/002` remain for the next part of #139. AC2 is not claimed.

The pinned Proxmox `internal/backend/proxmox/connection.go` forces
`cfg.ReadOnly = true`; its mutation methods return Unsupported regardless of
the YAML flag. Keeping the audit-only token and read-only region is therefore
required for this pin. The creating group still executes. Ten restored cases
stop at subnet discovery because the SigV4 region is lost, tracked by
https://github.com/popododo0720/vws-console-api/issues/792 with `local-pve`
entries. Termination does not reach Proxmox in this run, so this evidence does
not claim to have exercised its missing terminated-state behavior.

Both targets return `InvalidParameterValue` for a missing ImageId where
`EC2-RUN-101` expects `MissingParameter`. Local-fake also returns a client-token
mismatch message outside the existing message contract in `EC2-RUN-202`.
These observed gaps are tracked by
https://github.com/popododo0720/vws-console-api/issues/1005; the mismatch entry
is scoped to `local-fake`, while local-pve records its earlier routing failure.
No expectation or cleanup rule was loosened.

The first concurrent run exhausted the API's default launch bucket (5 tokens,
refill 2/s), producing 503 RequestLimitExceeded and unknown-create cleanup
errors. The disposable local API processes use their supported
`-ec2-launch-burst 100 -ec2-launch-rate 100` options so the suite can exercise
parity without incidental throttling. The ownership gate still refuses to
retry or discard an unknown create. The final run has zero such errors.

| Group / invocation | Pass | Untracked fail | Known fail | Skip | Error |
| --- | ---: | ---: | ---: | ---: | ---: |
| ec2/instances, read-only | 1 | 0 | 2 | 0 | 0 |
| ec2/images | 0 | 0 | 2 | 0 | 0 |
| ec2/keypairs | 0 | 0 | 1 | 0 | 0 |
| ec2/addresses | 0 | 0 | 1 | 0 | 0 |
| ec2/gaps | 1 | 0 | 2 | 0 | 0 |
| ebs/volumes | 0 | 0 | 1 | 0 | 0 |
| ebs/snapshots | 0 | 0 | 1 | 0 | 0 |
| ebs/gaps | 1 | 0 | 0 | 0 | 0 |
| vpc/vpcs | 0 | 0 | 1 | 0 | 0 |
| vpc/subnets | 0 | 0 | 1 | 0 | 0 |
| vpc/securitygroups | 0 | 0 | 1 | 0 | 0 |
| vpc/gaps | 3 | 0 | 0 | 0 | 0 |
| ec2/instances, full group | 1 | 0 | 13 | 0 | 0 |
| iam/users | 0 | 0 | 0 | 4 | 0 |
| iam/roles | 4 | 0 | 0 | 0 | 0 |
| iam/policies | 7 | 0 | 0 | 0 | 0 |
| iam/mfa | 2 | 0 | 1 | 1 | 0 |
| iam/policytags | 8 | 0 | 2 | 1 | 0 |
| iam/mfatags | 1 | 0 | 0 | 9 | 0 |
| iam/mfadevices | 0 | 0 | 0 | 1 | 0 |
| sts/caller | 1 | 0 | 0 | 1 | 0 |
| auth/sigv4 | 6 | 0 | 1 | 3 | 0 |
| Total invocations | 36 | 0 | 30 | 20 | 0 |

The first three read-only instance cases are deliberately repeated in the
full instances invocation; the table counts executed cases, not unique IDs.
The full group's run ID is `conf-20261113T090915Z-f5b208`.
Logs and reports are saved under `~/evidence/vws-conformance-139/`:
`local-pve-final.log`, `local-fake-final.log`, and `reports/`.
The workflow's full local-fake selection also exits **0**: 90 pass,
9 linked known failures, 10 existing skips, and zero untracked failures or
harness errors. Its instances group has 12 pass and 2 known failures
(`conf-20261113T091018Z-218265`).

```sh
export RUNNER_TEMP="$PWD/.scratch"
export TMPDIR="$RUNNER_TEMP"
export STACK_BIN="$RUNNER_TEMP/console-api/bin"
export CONSOLE_API_PIN=05840905e958f6ad5e57e200931d542798b22d73
export PATH="$RUNNER_TEMP/awscli/bin:$RUNNER_TEMP/etcd/usr/bin:$PATH"
bash scripts/local-pve.sh
```

The local stack was built in separate detached worktrees at the exact CI
pins, with the same etcd extraction and IAM bootstrap as the workflow.
`TestInstanceCasesRegistered` fails on the original main snapshot for all 11
missing IDs (`main-negative.log`). The local protocol fake exercises all 14
instance cases, cleanup after an assertion failure, repeated cleanup, and
idempotent launch replay. Foreign ownership blocks Stop, Start and Terminate
in `internal/harness/instance_gate_test.go`. The baseline AC1/AC3 command
`go test -race -count=1 -p 1 ./internal/harness -run 'TestMutatingSDKMethodsStayInAct|Instance|KeyPair'`
passed before the changes (`main-gates.log`). Removing the real RUN-202
exception in a unit test restores a parity-gate failure.

`go build ./...`, `go vet ./...`, `golangci-lint run --timeout=5m ./...`
(`0 issues`), and `go test -race -count=1 -p 1 ./...` all passed
(`quality.log`). `bash -n scripts/local-pve.sh` passed. Actionlint reports
only the pre-existing local-fake readiness-loop SC2034 warning, identical on
the main snapshot (`workflow-check.log`).

## Issue #138: passing run with approved ownership exclusions

The full local-pve command now exits **0**. All six service packages pass
with 35 passing cases, 17 linked known parity failures, 20 skips (10 existing
and 10 approved here), and zero harness errors or untracked failures.
IAM-MTAG-103 remains active and passes. The console and IAM pins are unchanged
from the initial run below.

The ten exclusions use `Case.Skip` only for target `local-pve`. Every skip
record carries its issue URL:

| Excluded case | Issue |
| --- | --- |
| IAM-MTAG-001 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-002 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-003 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-101 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-102 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-104 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-105 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-106 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-MTAG-107 | https://github.com/popododo0720/vws-iam/issues/501 |
| IAM-GAP-005 | https://github.com/popododo0720/vws-iam/issues/501 |

All ten cases come back when vws-iam#501 lands and the IAM pin moves to a
commit containing that fix. No guard or expectation changed. The redundant
local-pve known-failure entries were removed. Each case retains exactly one
unscoped console-api#850 record for the other targets, so local-fake's
selection and existing known failures are unchanged.

`TestLocalPVESkipsExactlyTenOwnershipCases` failed before expanding the
exclusions (`RUNNER_TEMP/ten-exclusions-negative.log`) and passed afterwards.
It compares every selected case, expectation and skip across all four
targets, repeats local-pve selection, and keeps IAM-MTAG-103 active.
`TestMFATagsKeepExistingKnownIssuesWithoutRedundantOverrides` checks the
unchanged mappings for the other targets and rejects redundant records.

The earlier exact-two attempt (`RUNNER_TEMP/local-pve-exclusions.log`) exited
1 on IAM-MTAG-102. It used a fresh etcd directory
`RUNNER_TEMP/local-pve.dwZ2eF/etcd` with `member-initialized: false`.
The remaining error was not state from a previous run: all creating MTAG
cases use the same `rt.Name("mtag")` within one run. After the first two skips,
IAM-MTAG-102 created the first device and IAM dropped its ownership tag.
Later cases then encountered that current-run device. This explains why all
nine creating MTAG cases depend on the upstream fix. Every script invocation
uses a new `mktemp` sandbox and its own etcd data directory.

The passing run's table separates known failures from untracked failures:

| Group | Pass | Untracked fail | Known fail | Skip | Error |
| --- | ---: | ---: | ---: | ---: | ---: |
| ec2/instances | 1 | 0 | 2 | 0 | 0 |
| ec2/images | 0 | 0 | 2 | 0 | 0 |
| ec2/keypairs | 0 | 0 | 1 | 0 | 0 |
| ec2/addresses | 0 | 0 | 1 | 0 | 0 |
| ec2/gaps | 1 | 0 | 2 | 0 | 0 |
| ebs/volumes | 0 | 0 | 1 | 0 | 0 |
| ebs/snapshots | 0 | 0 | 1 | 0 | 0 |
| ebs/gaps | 1 | 0 | 0 | 0 | 0 |
| vpc/vpcs | 0 | 0 | 1 | 0 | 0 |
| vpc/subnets | 0 | 0 | 1 | 0 | 0 |
| vpc/securitygroups | 0 | 0 | 1 | 0 | 0 |
| vpc/gaps | 3 | 0 | 0 | 0 | 0 |
| iam/users | 0 | 0 | 0 | 4 | 0 |
| iam/roles | 4 | 0 | 0 | 0 | 0 |
| iam/policies | 7 | 0 | 0 | 0 | 0 |
| iam/mfa | 2 | 0 | 1 | 1 | 0 |
| iam/policytags | 8 | 0 | 2 | 1 | 0 |
| iam/mfatags | 1 | 0 | 0 | 9 | 0 |
| iam/mfadevices | 0 | 0 | 0 | 1 | 0 |
| sts/caller | 1 | 0 | 0 | 1 | 0 |
| auth/sigv4 | 6 | 0 | 1 | 3 | 0 |
| Total | 35 | 0 | 17 | 20 | 0 |

```sh
cd /home/ubuntu/work/vws/.worktrees/vws-conformance/issue-138
export RUNNER_TEMP=/home/ubuntu/work/vws/.worktrees/vws-conformance/issue-138-tmp
export STACK_BIN="$RUNNER_TEMP/console-api/bin"
export CONSOLE_API_PIN=05840905e958f6ad5e57e200931d542798b22d73
export PATH="$RUNNER_TEMP/awscli-venv/bin:$RUNNER_TEMP/etcd/usr/bin:$PATH"
bash scripts/local-pve.sh >"$RUNNER_TEMP/local-pve-green.log" 2>&1
```

The full log remains at `RUNNER_TEMP/local-pve-green.log`. JSON reports are
preserved in `RUNNER_TEMP/green-out/`, with run IDs
`conf-20261112T220500Z-6d6bf8` (EC2), `conf-20261112T220501Z-3afe23` (EBS),
`conf-20261112T220502Z-de916b` (VPC), `conf-20261112T220504Z-36c723` (IAM),
`conf-20261112T220506Z-94617c` (STS), and `conf-20261112T220507Z-cb4e39` (AUTH).

Repository checks on the approved exclusion change all exited 0:
`sh scripts/guard.sh`, `go vet ./...`,
`go test -race -count=1 -p 1 ./internal/... ./cmd/...`,
`golangci-lint run --timeout=5m ./...` (`0 issues`),
`scripts/check-test-rand.sh`, `scripts/check-english.sh files`,
`golangci-lint fmt --diff`, and `git diff --check`.
The race run used the real AWS CLI from `RUNNER_TEMP/awscli-venv/bin` on PATH;
the harness package passed in 74.734 seconds. Full output is retained in
`RUNNER_TEMP/ten-exclusions-checks.log`.

## Issue #138: initial etcd-backed IAM, STS and AUTH run

Run on November 12, 2026 with console-api
`05840905e958f6ad5e57e200931d542798b22d73` (unchanged) and vws-iam
`d01a814a91afba7d884d9f5b0bf4ad7a23110a91` (the fetched `origin/main`,
verified with `git rev-parse`). Built the four existing console stack
executables plus `cmd/vws-iam`. Extracted Ubuntu etcd-server 3.5.16 without
installing or starting a system service.

The script generates a CA and server/client certificates with only the
127.0.0.1 SAN. Both IAM and the admin registry authenticate to the same etcd
member with client certificates. Console-api connects to IAM with mutual TLS;
region traffic retains mutual TLS. The etcd health endpoint is on its
separate loopback metrics listener, so every readiness and setup HTTP request
still uses the guarded `conf-probe`.

The public conformance credential remains AKIDEXAMPLE with the synthetic
development secret and account 123456789012. The pinned console's
`iam.WithRootKey` authenticates that root key locally and sends created
identities and management calls to IAM. IAM bootstrap itself requires a valid
20-character AKIA key ID and at least 32 secret bytes, so the script generates
a separate bootstrap credential in the same account. It imports it twice to
check idempotence. Neither the random credential nor its secret is logged.

```sh
cd /home/ubuntu/work/vws/.worktrees/vws-conformance/issue-138
export RUNNER_TEMP=/home/ubuntu/work/vws/.worktrees/vws-conformance/issue-138-tmp
export STACK_BIN="$RUNNER_TEMP/console-api/bin"
export CONSOLE_API_PIN=05840905e958f6ad5e57e200931d542798b22d73
export PATH="$RUNNER_TEMP/etcd/usr/bin:$PATH"
bash scripts/local-pve.sh >"$RUNNER_TEMP/local-pve-verified.log" 2>&1
```

The end-to-end command exits **1**, with no untracked parity failures.
IAM-MTAG-107 and IAM-GAP-005 have hard ownership/cleanup errors:
vws-iam's `internal/grpcapi/mfa.go` CreateVirtualMfaDevice never reads the
request Tags. Its successful creation therefore drops the run ownership tag,
and the conformance guard correctly preserves the device. Later MFA tag cases
encounter EntityAlreadyExists on setup. This is an upstream defect, filed
after duplicate searches as
https://github.com/popododo0720/vws-iam/issues/501.
Known-issue records do not suppress harness errors, and no ownership rule,
cleanup check, expectation or existing skip was weakened.

The ten local-pve entries for IAM-MTAG-001/002/003/101/102/104/105/106/107 and
IAM-GAP-005 link to IAM#501. Target-specific entries override the unscoped
entries regardless of file order, leaving local-fake's console-api#850
tracker intact. Unit tests cover both orders, all four targets, duplicate
rejection and the actual MFA mappings. Existing malformed/excluded-target
validation still fails closed.

Final JSON report counts (known failures are also failures in the raw JSON;
the columns below separate them from untracked failures):

| Group | Pass | Untracked fail | Known fail | Skip | Error |
| --- | ---: | ---: | ---: | ---: | ---: |
| ec2/instances | 1 | 0 | 2 | 0 | 0 |
| ec2/images | 0 | 0 | 2 | 0 | 0 |
| ec2/keypairs | 0 | 0 | 1 | 0 | 0 |
| ec2/addresses | 0 | 0 | 1 | 0 | 0 |
| ec2/gaps | 1 | 0 | 2 | 0 | 0 |
| ebs/volumes | 0 | 0 | 1 | 0 | 0 |
| ebs/snapshots | 0 | 0 | 1 | 0 | 0 |
| ebs/gaps | 1 | 0 | 0 | 0 | 0 |
| vpc/vpcs | 0 | 0 | 1 | 0 | 0 |
| vpc/subnets | 0 | 0 | 1 | 0 | 0 |
| vpc/securitygroups | 0 | 0 | 1 | 0 | 0 |
| vpc/gaps | 3 | 0 | 0 | 0 | 0 |
| iam/users | 0 | 0 | 0 | 4 | 0 |
| iam/roles | 4 | 0 | 0 | 0 | 0 |
| iam/policies | 7 | 0 | 0 | 0 | 0 |
| iam/mfa | 2 | 0 | 1 | 1 | 0 |
| iam/policytags | 8 | 0 | 2 | 1 | 0 |
| iam/mfatags | 1 | 0 | 8 | 0 | 1 |
| iam/mfadevices | 0 | 0 | 0 | 0 | 1 |
| sts/caller | 1 | 0 | 0 | 1 | 0 |
| auth/sigv4 | 6 | 0 | 1 | 3 | 0 |
| Total | 35 | 0 | 25 | 10 | 2 |

Reports were generated under `out/conf-20261112T214533Z-38eef6` (EC2),
`out/conf-20261112T214534Z-db68ba` (EBS),
`out/conf-20261112T214534Z-f7f884` (VPC),
`out/conf-20261112T214536Z-7fc778` (IAM),
`out/conf-20261112T214538Z-df21a4` (STS), and
`out/conf-20261112T214539Z-ebb07c` (AUTH).
The generated `out/` directory was moved to `RUNNER_TEMP/out/` after the run
to keep the source worktree clean. The full local log stays at the
RUNNER_TEMP path above for the orchestrator.

All runtime service endpoints, peers, health listeners and Proxmox URLs use
127.0.0.1; `VWS_CONF_ALLOWED_HOSTS` remains exactly 127.0.0.1. After readiness,
`ss -Hltnp` checks the actual started PIDs, requires a visible TCP listener
for each daemon, and refuses any address except 127.0.0.1 or [::1].
The actual awk predicate also passed six boundary fixtures: IPv4 and IPv6
loopback accepted; IPv4 wildcard, IPv6 wildcard, non-loopback, and a different
PID rejected. The fixture output is `RUNNER_TEMP/listener-boundary.log`.
No suite request is sent to a non-loopback host; source/package downloads
remain build prerequisites. Existing egress guard tests remain in the job.

The script has been rerun with fresh stores, and bootstrap is repeated within
each run. Power-loss recovery of the unchanged upstream store/daemons remains
upstream coverage. No vws-test cluster run was made: this issue explicitly
requires loopback-only local evidence.

Repository verification for this revision:

- Pinned stack builds: exit 0. Local Go is 1.26.10; the workflow keeps 1.26.9.
- `sh scripts/guard.sh`, `go vet ./...`, `scripts/check-test-rand.sh`,
  `scripts/check-english.sh files`, `bash -n scripts/local-pve.sh`,
  `golangci-lint run --timeout=5m ./...`, and `golangci-lint fmt --diff`:
  exit 0. Lint reports `0 issues`; formatting has no diff.
- `go test -race -count=1 -p 1 ./internal/... ./cmd/...`: every package passed
  except `internal/harness`, initially missing the real AWS CLI prerequisite.
  A self-contained disposable virtual environment then provided AWS CLI
  1.46.3. With that environment's `bin` on PATH,
  `go test -race -count=1 -p 1 ./internal/harness` passed in 70.584 seconds.
  The real CLI subprocess uses the existing guarded proxy and sanitized
  credentials/environment. No test was skipped or weakened.
- `go test -race -count=1 -p 1 ./internal/engine` also passed after the final
  workflow assertion change.
- `actionlint`: exit 1 on pre-existing workflow findings. Checking only
  conformance.yml produces SC2034 for the unchanged local-fake readiness
  loop variable; the same check on the original HEAD produces that exact
  finding. The full check also reports existing verify-queue.yml shellcheck
  findings. No new local-pve diagnostic was reported.
- `git diff --check`: exit 0.

Verification logs remain in RUNNER_TEMP: `checks-final.log` (the initial
complete package run and lint), `checks-amended.log` (final guard and engine
assertion), `harness-verified.log` (successful guarded CLI rerun), and
`actionlint-baseline.log` (unchanged conformance workflow diagnostic).
The isolated CLI was installed with
`python3 -m venv --without-pip "$RUNNER_TEMP/awscli-venv"` and a disposable
pip zipapp's `--python "$RUNNER_TEMP/awscli-venv/bin/python" install awscli`.
The verification rerun was:

```sh
PATH="$RUNNER_TEMP/awscli-venv/bin:$PATH" \
  go test -race -count=1 -p 1 ./internal/harness
```

## Earlier issue #137 evidence

Run on November 12, 2026 with console-api
`05840905e958f6ad5e57e200931d542798b22d73`, the same pin as the workflow.

Built `cmd/pvefake`, `cmd/vws-region`, `cmd/vws-console-api`, and the upstream
`internal/regionhelper` test executable. Extracted Ubuntu's `etcd-server`
package without installing or starting a system service.

```sh
export TMPDIR=/home/ubuntu/work/vws/.worktrees/vws-conformance/.tmp-137
export RUNNER_TEMP="$TMPDIR"
export STACK_BIN="$TMPDIR/console-api/bin"
export CONSOLE_API_PIN=05840905e958f6ad5e57e200931d542798b22d73
export PATH="$TMPDIR/etcd/usr/bin:$PATH"
bash scripts/local-pve.sh >.tmp/local-pve-verified.log 2>&1
```

Exit status: 0. All four service packages passed. The readiness checks passed
for etcd, pvefake, the region daemon and the global API. Admin registration
succeeded; the console inventory request returned the scenario's instance
through the global regions backend and the mTLS region API. Re-running the
script used a fresh sandbox and terminated all child processes on exit.

The JSON reports contain 29 selected cases: 12 pass, 14 known parity failures,
3 existing IAM-dependent AUTH skips, and 0 harness errors. The initial run
failed on all 14 untracked cases before the known-issue entries were added.
The successful run reported every failure with its issue URL.

All 14 failures share the credential-region routing gap in
https://github.com/popododo0720/vws-console-api/issues/792.
The verifier checks the signed region but returns a Caller without it;
regionset receives an empty Scope.Region and refuses the read. Reproduction
and explicit acceptance criteria were added to that existing issue after
duplicate searches. The console inventory smoke check uses an explicit region
to prove the full stack independently of that known SigV4 gap.

`internal/engine/read_only_test.go` covers complete service counts, selection
inside a group containing mutations, unknown input, empty input, idempotent
filtering, and preserving the full suite. The filter adds no durable state:
power-loss recovery of the daemon, registry, and credential helper remains
upstream behavior. The existing harness and binary egress tests run in
`local-pve`, with strace installed; the endpoint allowlist remains
`127.0.0.1` only.

The workflow uploads JSON reports and process logs even on failure. Local raw
logs were captured in `.tmp/` for review and are not committed.

The workflow regression assertion was also run against an archived
`origin/main` snapshot: it failed with `workflow has no local-pve job`.
`TestCIRunsLocalPVE` passes on this branch.

Repository verification:

- `golangci-lint run --timeout=5m ./...`: exit 0.
- `golangci-lint fmt --diff` and `bash -n scripts/local-pve.sh`: exit 0.
- `scripts/guard.sh`, `scripts/guard_test.sh`,
  `scripts/check-test-rand.sh`, `scripts/check-test-rand_test.sh`: exit 0.
- `go test -race -count=1 -timeout 300s ./...`: every package passed except
  `internal/harness`, whose CLI integration test required a missing `aws`
  executable. Installed the real AWS CLI from PyPI in a disposable virtual
  environment, then ran
  `go test -race -count=1 -timeout 300s ./internal/harness` with that directory
  on PATH: exit 0 (`ok`, 70.496s). Every package from the full race run is now
  verified; no test was changed or skipped to handle the missing prerequisite.

After review, the 14 Proxmox exceptions were scoped to `local-pve`.
`TestKnownIssuesStayInTheirTarget` checks all 14 on all four targets and keeps
unscoped exceptions working. `TestKnownIssuesValidateExcludedTargets` refuses
malformed records even when their target is excluded. The negative assertion
failed on the original PR head with `Proxmox-only exception masks local-fake`.
The corrected full race command exited 0, lint reported `0 issues`, formatting
had no diff, and the complete local-pve script exited 0 again with the same
12 pass / 14 known / 3 skip / 0 error result.

The final revision replaces every setup request with `cmd/conf-probe`, using
the existing `harness.NewHTTPClient` and endpoint check. HTTPS readiness uses
the generated CA through `SSL_CERT_FILE`. The guarded script, complete race
suite, lint, formatting and repository guards all exited 0 after this change.
The workflow test rejects direct curl probes; helper tests cover request
headers/body, repeat execution, invalid inputs, HTTP failures and redirects.
Incident disclosure: https://github.com/popododo0720/vws-conformance/issues/157.

Additional manual guard check:

```sh
go build -o .tmp/conf-probe ./cmd/conf-probe
strace -f -e trace=connect,sendto,sendmsg,sendmmsg \
  -e inject=connect,sendto,sendmsg,sendmmsg:error=ENETUNREACH \
  -o .tmp/probe-egress.trace .tmp/conf-probe http://192.0.2.1
```

The probe exited 1 with `endpoint host "192.0.2.1" is refused`. The trace had
no `AF_INET` syscall: rejection preceded all IP networking, with network
syscalls disabled during the proof.
