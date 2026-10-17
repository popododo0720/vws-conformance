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
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy
unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_ENDPOINT_URL AWS_ENDPOINT_URL_S3
if ! command -v aws >/dev/null 2>&1; then
  go run ./cmd/conf-run check-endpoint
  echo "skip: aws cli is not installed"
  exit 0
fi
go run ./cmd/conf-run cli s3api list-buckets >/dev/null
set +e
go run ./cmd/conf-run cli s3api head-bucket --bucket "no-such-bucket-conformance" >/dev/null 2>/tmp/conf-cli-err
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
