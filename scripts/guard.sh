#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

fail() {
  echo "guard: $1" >&2
  exit 1
}

if grep -R -n 'github.com/popododo0720/vws-' --include='*.go' --include='go.mod' . | grep -v 'github.com/popododo0720/vws-conformance'; then
  fail "this module must not import another vws module"
fi

if grep -R -n -E '^[[:space:]]*schedule:' .github/workflows; then
  fail "workflows must not use a schedule trigger"
fi

if grep -R -n -E 'https?://[^[:space:]]*amazonaws\.com' --include='*.go' --include='*.yml' --include='*.sh' . \
  | grep -v '_test\.go'; then
  fail "an endpoint URL uses amazonaws.com"
fi

if grep -R -n -E 'AKIA[0-9A-Z]{16}' --include='*.go' --include='*.yml' --include='*.md' --include='*.sh' --include='*.json' . \
  | grep -v 'AKIAUNKNOWN00000000'; then
  fail "possible access key"
fi

if grep -R -n -P '[\x{AC00}-\x{D7A3}]' --include='*.go' --include='*.md' --include='*.yml' --include='*.sh' .; then
  fail "non-English text"
fi

echo "guard ok"
