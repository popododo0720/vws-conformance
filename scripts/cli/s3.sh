#!/bin/sh
# One S3 happy path and one S3 error path through the AWS CLI.
# The caller exports VWS_CONF_* and has aws on PATH. set +x is required.
set -eu
set +x
if ! command -v aws >/dev/null 2>&1; then
  echo "skip: aws cli is not installed"
  exit 0
fi
: "${VWS_CONF_S3_ENDPOINT:?}"
: "${VWS_CONF_ACCESS_KEY_ID:?}"
: "${VWS_CONF_SECRET_ACCESS_KEY:?}"
export AWS_ACCESS_KEY_ID="$VWS_CONF_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$VWS_CONF_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION=vws-1
export AWS_EC2_METADATA_DISABLED=true
aws s3api list-buckets --endpoint-url "$VWS_CONF_S3_ENDPOINT" >/dev/null
if aws s3api head-bucket --endpoint-url "$VWS_CONF_S3_ENDPOINT" --bucket "no-such-bucket-conformance" >/dev/null 2>/tmp/conf-cli-err; then
  echo "head-bucket on a missing bucket succeeded" >&2
  exit 1
fi
echo "cli s3 happy list-buckets and error head-bucket"
