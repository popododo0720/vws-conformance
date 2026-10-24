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

allowlist=.github/english-allowlist
if [ ! -f "$allowlist" ]; then
  allowlist=/dev/null
fi
allowed() {
  path=$1
  path=${path#./}
  while read -r glob _; do
    case $glob in
      ''|'#'*) continue ;;
    esac
    case $path in
      $glob) return 0 ;;
    esac
  done < "$allowlist"
  return 1
}
hits=$(mktemp)
trap 'rm -f "$hits"' EXIT
grep -R -n -P '[\x{AC00}-\x{D7A3}]' --include='*.go' --include='*.md' --include='*.yml' --include='*.sh' . >"$hits" || true
bad=0
while IFS= read -r line; do
  [ -n "$line" ] || continue
  file=${line%%:*}
  if allowed "$file"; then
    continue
  fi
  printf '%s\n' "$line"
  bad=1
done < "$hits"
if [ "$bad" -ne 0 ]; then
  fail "non-English text"
fi

# R21 (popododo0720/vws-deploy#448): AGENTS.md has a non-empty '## /correct loop' section, the incident
# form applies the incident label with every required field, and .github/labels.yml defines the label.
# scripts/guard_test.sh shows this fails on an empty section.
bash scripts/guard-agents.sh correct . ||
  fail "R21: the /correct loop, incident form or incident label is missing (AGENTS.md '## /correct loop', .github/ISSUE_TEMPLATE/incident.yml, .github/labels.yml)"

echo "guard ok"
