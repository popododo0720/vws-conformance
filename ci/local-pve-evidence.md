# Local Proxmox job evidence

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
