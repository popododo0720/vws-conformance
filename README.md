# vws-conformance
Unmodified AWS SDK/CLI conformance suite for VWS

The `local-pve` conformance job builds console-api at the workflow's pinned
commit, starts a YAML-backed HTTPS `pvefake`, a Proxmox region daemon and the
global regions backend, and registers `pve-test-1` through the admin API.
The registry uses a disposable loopback etcd with TLS. The region daemon keeps
its registration in a private `-state-dir` (mode 0700, under the system temp
directory so every ancestor is safe) and needs no `/etc/pve`, helper or KEK.
The registration carries `pveApiUrls` and `caPem`.

Run the job locally with `etcd`, `openssl`, `jq` and `ss` (iproute2) on PATH and these four
executables in `STACK_BIN`, built at the pins in
`.github/workflows/conformance.yml` (`CONSOLE_API_PIN` in the workflow `env`,
`IAM_PIN` in the `local-pve` job):

- `pvefake`, `vws-region` and `vws-console-api` from vws-console-api at `CONSOLE_API_PIN`
- `vws-iam` from vws-iam at `IAM_PIN`

The block runs in a subshell with `set -eu` and clones into a fresh `mktemp -d`
directory, so a failed clone, checkout or build stops it before the next step,
and no older checkout is ever built.

```sh
(
set -eu
CONSOLE_API_PIN=16c0135c7f4af3c318f893501146eca1fe533404
IAM_PIN=d01a814a91afba7d884d9f5b0bf4ad7a23110a91
STACK_BIN=/absolute/path/to/stack/bin
src=$(mktemp -d)
git clone https://github.com/popododo0720/vws-console-api.git "$src/console-api"
git -C "$src/console-api" checkout --detach "$CONSOLE_API_PIN"
test "$(git -C "$src/console-api" rev-parse HEAD)" = "$CONSOLE_API_PIN"
for cmd in pvefake vws-region vws-console-api; do
  go -C "$src/console-api" build -o "$STACK_BIN/$cmd" "./cmd/$cmd"
done
git clone https://github.com/popododo0720/vws-iam.git "$src/iam"
git -C "$src/iam" checkout --detach "$IAM_PIN"
test "$(git -C "$src/iam" rev-parse HEAD)" = "$IAM_PIN"
go -C "$src/iam" build -o "$STACK_BIN/vws-iam" ./cmd/vws-iam
RUNNER_TEMP=/absolute/path/outside/module STACK_BIN="$STACK_BIN" \
CONSOLE_API_PIN="$CONSOLE_API_PIN" bash scripts/local-pve.sh
)
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
