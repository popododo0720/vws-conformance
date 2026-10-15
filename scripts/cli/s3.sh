#!/bin/sh
set -eu
set +x
if [ -n "${AWS_PROFILE:-}" ] || [ -n "${AWS_DEFAULT_PROFILE:-}" ]; then
  echo "AWS_PROFILE is refused" >&2
  exit 1
fi
: "${VWS_CONF_S3_ENDPOINT:?}"
: "${VWS_CONF_ACCESS_KEY_ID:?}"
: "${VWS_CONF_SECRET_ACCESS_KEY:?}"
go run ./cmd/conf-run check-endpoint
if ! command -v aws >/dev/null 2>&1; then
  echo "skip: aws cli is not installed"
  exit 0
fi
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN AWS_SECURITY_TOKEN
export AWS_SHARED_CREDENTIALS_FILE=/dev/null
export AWS_CONFIG_FILE=/dev/null
export AWS_ACCESS_KEY_ID="$VWS_CONF_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$VWS_CONF_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION=vws-1
export AWS_EC2_METADATA_DISABLED=true
if [ -n "${VWS_CONF_SESSION_TOKEN:-}" ]; then
  export AWS_SESSION_TOKEN="$VWS_CONF_SESSION_TOKEN"
fi
aws s3api list-buckets --endpoint-url "$VWS_CONF_S3_ENDPOINT" >/dev/null
set +e
aws s3api head-bucket --endpoint-url "$VWS_CONF_S3_ENDPOINT" --bucket "no-such-bucket-conformance" >/dev/null 2>/tmp/conf-cli-err
code=$?
set -e
if [ "$code" -eq 0 ]; then
  echo "head-bucket on a missing bucket succeeded" >&2
  exit 1
fi
if [ "$code" -eq 127 ] || [ "$code" -eq 253 ]; then
  echo "head-bucket invocation failed" >&2
  exit 1
fi
err=$(cat /tmp/conf-cli-err 2>/dev/null || true)
case "$err" in
  *404*|*"Not Found"*|*NoSuchBucket*) ;;
  *)
    echo "head-bucket did not return 404" >&2
    exit 1
    ;;
esac
echo "cli s3 happy list-buckets and error head-bucket"
