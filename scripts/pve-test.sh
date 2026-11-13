#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

: "${VWS_CONF_ALLOWED_HOSTS:?set the nested box host allowlist}"
: "${VWS_CONF_EC2_ENDPOINT:?set the nested box VWS EC2 endpoint}"
: "${VWS_CONF_IAM_ENDPOINT:?set the nested box VWS IAM endpoint}"
: "${VWS_CONF_REGION:?set the nested box region}"
: "${VWS_CONF_ACCOUNT_ID:?set the throwaway VWS account}"
: "${VWS_CONF_ACCESS_KEY_ID:?set the throwaway VWS access key}"
: "${VWS_CONF_SECRET_ACCESS_KEY:?set the throwaway VWS secret}"
: "${VWS_CONF_OUT:?set the persistent report directory}"

export VWS_CONF_TARGET=pve-test
export VWS_CONF_ALLOWED_ACCOUNTS="$VWS_CONF_ACCOUNT_ID"
exec go run ./cmd/conf-run -full -read-only
