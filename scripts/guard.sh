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

# vws-conformance#35: GitHub runs every job of a workflow on each of its events, so a file on both
# pull_request and pull_request_target records a skipped twin of each job, and a skipped required check
# parks the pull request. scripts/guard_test.sh shows this fails on such a file.
for wf in .github/workflows/*.yml .github/workflows/*.yaml; do
  [ -f "$wf" ] || continue
  # Only the top-level on: block counts, so a step that mentions an event name is not a trigger.
  events=$(sed 's/#.*//' "$wf" | awk '
    /^["'"'"']?on["'"'"']?[[:space:]]*:/ { on = 1; sub(/^[^:]*:/, ""); print; next }
    on && /^[^[:space:]]/ { on = 0 }
    on { print }')
  if printf '%s\n' "$events" | grep -q -E '(^|[^A-Za-z0-9_])pull_request([^A-Za-z0-9_]|$)' &&
    printf '%s\n' "$events" | grep -q -E '(^|[^A-Za-z0-9_])pull_request_target([^A-Za-z0-9_]|$)'; then
    fail "dual-event: ${wf} runs on both pull_request and pull_request_target; split it into one file per event"
  fi
done

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
