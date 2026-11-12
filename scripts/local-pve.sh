#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
: "${STACK_BIN:?set STACK_BIN to the pinned console-api bin directory}"
: "${RUNNER_TEMP:?set RUNNER_TEMP outside the Go module}"
work=$(mktemp -d "$RUNNER_TEMP/local-pve.XXXXXX")
probe="$work/conf-probe"
go build -o "$probe" ./cmd/conf-probe
mkdir -p "$work"/{public,private,locks}
umask 077
pids=()
cleanup() {
  status=$?
  trap - EXIT
  if ((${#pids[@]})); then
    kill "${pids[@]}" 2>/dev/null || true
    for pid in "${pids[@]}"; do wait "$pid" || true; done
  fi
  if [ "$status" -ne 0 ]; then
    cat "$work"/*.log
  fi
  exit "$status"
}
trap cleanup EXIT
wait_ready() {
  local url=$1
  shift
  for ((i=0; i<30; i++)); do
    if "$probe" "$@" "$url" >/dev/null 2>&1; then return; fi
    sleep 1
  done
  echo "readiness timed out: $url" >&2
  return 1
}

openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -subj /CN=local-pve -addext subjectAltName=IP:127.0.0.1 \
  -keyout "$work/key.pem" -out "$work/cert.pem" 2>"$work/tls.log"
openssl rand -base64 32 >"$work/kek"
openssl rand 32 >"$work/region-kek"
cat >"$work/global.yaml" <<EOF
iam:
  etcd_endpoints: [https://127.0.0.1:2379]
  tls: {ca_file: $work/cert.pem}
credentials:
  kek_file: $work/kek
  mtls: {ca_file: $work/cert.pem, cert_file: $work/cert.pem, key_file: $work/key.pem}
default_project: "123456789012"
EOF
printf 'read_only: true\n' >"$work/region.yaml"
# The upstream test helper runs the real encrypted-file implementation under this sandbox root.
export VWS_HELPER_TEST_ROOT="$work" CREDENTIALS_DIRECTORY="$work"
etcd --name local-pve --data-dir "$work/etcd" \
  --listen-client-urls https://127.0.0.1:2379 --advertise-client-urls https://127.0.0.1:2379 \
  --listen-peer-urls http://127.0.0.1:2380 --initial-advertise-peer-urls http://127.0.0.1:2380 \
  --initial-cluster local-pve=http://127.0.0.1:2380 \
  --cert-file "$work/cert.pem" --key-file "$work/key.pem" >"$work/etcd.log" 2>&1 &
pids+=("$!")
SSL_CERT_FILE="$work/cert.pem" wait_ready https://127.0.0.1:2379/health
"$STACK_BIN/pvefake" -listen 127.0.0.1:8006 -scenario ci/pve-scenario.yaml \
  -ca-file "$work/pve-ca.pem" >"$work/pvefake.log" 2>&1 &
pids+=("$!")
SSL_CERT_FILE="$work/pve-ca.pem" wait_ready https://127.0.0.1:8006/api2/json/version \
  -authorization 'PVEAPIToken=vws@pve!inventory=fake-secret'
"$STACK_BIN/vws-region" -backend proxmox -config "$work/region.yaml" \
  -registration-file "$work/public/registration.json" -helper-path "$STACK_BIN/region-helper" \
  -pve-endpoint https://127.0.0.1:8006 -pve-ca-file "$work/pve-ca.pem" \
  -tls-cert-file "$work/cert.pem" -tls-key-file "$work/key.pem" -tls-ca-file "$work/cert.pem" \
  -region-addr 127.0.0.1:8443 -ops-addr 127.0.0.1:8085 >"$work/region.log" 2>&1 &
pids+=("$!")
wait_ready http://127.0.0.1:8085/healthz
"$STACK_BIN/vws-console-api" -backend regions -iam fake -default-region pve-test-1 \
  -global-config "$work/global.yaml" -console-token dev-token -access-key AKIDEXAMPLE -secret secret \
  -console-addr 127.0.0.1:8080 -ec2-addr 127.0.0.1:8081 -s3-addr 127.0.0.1:8082 \
  -iam-addr 127.0.0.1:8084 -ops-addr 127.0.0.1:8083 >"$work/console-api.log" 2>&1 &
pids+=("$!")
wait_ready http://127.0.0.1:8083/healthz
"$probe" -method POST -authorization 'Bearer dev-token' \
  http://127.0.0.1:8080/api/v1/admin/regions >"$work/registration.log" <<'JSON'
{"name":"pve-test-1","clusterName":"test","pveApiUrl":"https://127.0.0.1:8006","tokenId":"vws@pve!inventory","tokenSecret":"fake-secret","regionEndpoint":"https://127.0.0.1:8443","zones":[{"letter":"a","nodes":["pve1"]}]}
JSON
wait_ready http://127.0.0.1:8085/readyz
"$probe" -authorization 'Bearer dev-token' -region pve-test-1 \
  http://127.0.0.1:8080/api/v1/instances >"$work/inventory.log"
jq -e '.items | length > 0' "$work/inventory.log"
export VWS_CONF_ACCESS_KEY_ID=AKIDEXAMPLE VWS_CONF_SECRET_ACCESS_KEY=secret
export VWS_CONF_ACCOUNT_ID=123456789012 VWS_CONF_ALLOWED_ACCOUNTS=123456789012
export VWS_CONF_EC2_ENDPOINT=http://127.0.0.1:8081 VWS_CONF_S3_ENDPOINT=http://127.0.0.1:8082
export VWS_CONF_IAM_ENDPOINT=http://127.0.0.1:8084 VWS_CONF_ALLOWED_HOSTS=127.0.0.1
export VWS_CONF_REGION=pve-test-1 VWS_CONF_TARGET=local-pve
export VWS_CONF_CONSOLE_API_VERSION="${CONSOLE_API_PIN:?set CONSOLE_API_PIN}"
go test -v -p 1 -count=1 -timeout 300s ./ec2 ./ebs ./vpc ./auth -args -full -read-only
