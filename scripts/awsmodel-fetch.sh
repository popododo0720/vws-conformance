#!/bin/sh
# Usage: scripts/awsmodel-fetch.sh <botocore-tag> <dir>
# Downloads, for every model listed in cmd/awsmodel-diff/pin.json, the botocore service-2.json at
# the given release tag into <dir>/<service>.json (vws-conformance#121). The files are public and
# come from github.com by tag, never from a branch; nothing here calls AWS.
set -eu
cd "$(dirname "$0")/.."

tag=${1:?usage: awsmodel-fetch.sh <botocore-tag> <dir>}
dir=${2:?usage: awsmodel-fetch.sh <botocore-tag> <dir>}
if ! printf '%s\n' "$tag" | grep -q -x -E '[0-9]+\.[0-9]+\.[0-9]+'; then
  echo "awsmodel-fetch: $tag is not a botocore release tag" >&2
  exit 2
fi
mkdir -p "$dir"
jq -r '.models[] | "\(.service) \(.path)"' cmd/awsmodel-diff/pin.json | while read -r service path; do
  curl -fsSL --proto '=https' --max-time 120 --retry 3 \
    -o "$dir/$service.json" "https://raw.githubusercontent.com/boto/botocore/$tag/$path"
done
