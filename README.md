# vws-conformance
Unmodified AWS SDK/CLI conformance suite for VWS

The `local-pve` conformance job builds console-api at the workflow's pinned
commit, starts a YAML-backed HTTPS `pvefake`, a Proxmox region daemon and the
global regions backend, and registers `pve-test-1` through the admin API.
The registry uses a disposable loopback etcd with TLS. Region credentials use
the upstream helper test binary, which runs the real encrypted-file operations
under a temporary sandbox root without requiring a Proxmox host.

Run the job locally after building the four binaries listed in
`.github/workflows/conformance.yml` and placing `etcd`, `openssl` and
`jq` on PATH:

```sh
RUNNER_TEMP=/absolute/path/outside/module \
STACK_BIN=/absolute/path/to/console-api/bin \
CONSOLE_API_PIN=05840905e958f6ad5e57e200931d542798b22d73 \
bash scripts/local-pve.sh
```

`-full -read-only` selects the 29 EC2, EBS, VPC and AUTH cases documented in
`docs/split-plan.md` that create no resources, including cases in groups that
also contain mutating fixtures. New cases are excluded until reviewed for this
selection. Existing IAM-dependent AUTH skips remain unchanged. Reports include
known parity failures with their issue links; harness errors and untracked
failures still fail the job. Process logs remain under `RUNNER_TEMP/local-pve.*`
and reports under `out/`; CI uploads both on failure.

Known-issue lines accept `CASE-ID ISSUE-URL [TARGET]`. A target limits an
exception to that environment; the Proxmox routing exceptions apply only to
`local-pve`, so the same cases still detect regressions on other targets.

The script builds `cmd/conf-probe` for readiness, registration and inventory
requests. It uses the existing guarded HTTP client with only `127.0.0.1`
allowed, refuses redirects and bounds requests to five seconds. HTTPS probes
trust the generated CA through `SSL_CERT_FILE`; TLS verification stays enabled.
