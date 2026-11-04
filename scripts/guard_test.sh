#!/usr/bin/env bash
# Tests for scripts/guard.sh correct (popododo0720/vws-deploy#448) and its dual-event check (#35): it passes on this repo's files and
# fails when the AGENTS.md /correct loop section, the incident form or the incident label is missing or
# empty. Each case runs on a throwaway copy in a temp dir; the work tree is never changed.
# Run: scripts/guard_test.sh
set -euo pipefail
export LC_ALL=C.UTF-8
root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0

copy() {
  local dest=$1 f
  mkdir -p "$dest/scripts" "$dest/.github/ISSUE_TEMPLATE"
  # .github/english-allowlist: a repo whose guard.sh also runs its English check reads it.
  for f in AGENTS.md scripts/guard.sh scripts/guard-agents.sh .github/ISSUE_TEMPLATE/incident.yml .github/labels.yml \
    .github/english-allowlist; do
    [[ -f $root/$f ]] && cp "$root/$f" "$dest/$f"
  done
  return 0
}

expect() {
  local want=$1 label=$2 dir=$3 rc=0
  (cd "$dir" && bash scripts/guard.sh correct) >"$tmp/out" 2>&1 || rc=$?
  # A failure counts only when guard-agents.sh correct names the cause, so a missing or broken
  # scripts/guard.sh cannot make a "fail" case pass.
  local ok=0
  if [[ $want == pass ]]; then
    [[ $rc -eq 0 ]] && ok=1
  elif [[ $rc -ne 0 ]] && grep -q '^correct: ' "$tmp/out"; then
    ok=1
  fi
  if [[ $ok -eq 1 ]]; then
    echo "ok: ${label} (${want})"
  else
    echo "FAIL: ${label}: want ${want}, exit ${rc}"
    sed 's/^/    /' "$tmp/out"
    fails=$((fails + 1))
  fi
}

case_dir() {
  local d="$tmp/$1"
  rm -rf "$d"
  copy "$d"
  printf '%s\n' "$d"
}

d=$(case_dir complete)
expect pass "this repo's /correct loop, incident form and incident label" "$d"

# Keep the heading, drop every body line up to the next # or ## heading.
d=$(case_dir empty)
if [[ -f $d/AGENTS.md ]]; then
  awk '$0 == "## /correct loop" { print; s = 1; next } s && /^##?([[:space:]]|$)/ { s = 0 } !s { print }' \
    "$d/AGENTS.md" >"$d/AGENTS.new" && mv "$d/AGENTS.new" "$d/AGENTS.md"
fi
expect fail "an empty /correct loop section" "$d"

d=$(case_dir no-heading)
[[ -f $d/AGENTS.md ]] && sed -i '/^## \/correct loop$/d' "$d/AGENTS.md"
expect fail "no /correct loop heading" "$d"

d=$(case_dir no-form)
rm -f "$d/.github/ISSUE_TEMPLATE/incident.yml"
expect fail "no incident form" "$d"

d=$(case_dir no-label)
[[ -f $d/.github/labels.yml ]] && sed -i '/^- name: incident$/d' "$d/.github/labels.yml"
expect fail "labels.yml without the incident label" "$d"

# vws-conformance#35: a workflow file on both pull_request and pull_request_target fails the guard,
# in either trigger form, and the guard's message names the file.
expect_dual() {
  local want=$1 label=$2 dir=$3 rc=0 ok=0
  (cd "$dir" && bash scripts/guard.sh) >"$tmp/out" 2>&1 || rc=$?
  if [[ $want == pass ]]; then
    [[ $rc -eq 0 ]] && ok=1
  elif [[ $rc -ne 0 ]] && grep -q '^guard: dual-event: .github/workflows/w.yml ' "$tmp/out"; then
    ok=1
  fi
  if [[ $ok -eq 1 ]]; then
    echo "ok: ${label} (${want})"
  else
    echo "FAIL: ${label}: want ${want}, exit ${rc}"
    sed 's/^/    /' "$tmp/out"
    fails=$((fails + 1))
  fi
}

dual_case() {
  local d
  d=$(case_dir "$1")
  mkdir -p "$d/.github/workflows"
  printf '%s\n' "$2" >"$d/.github/workflows/w.yml"
  printf '%s\n' "$d"
}

d=$(dual_case dual-map $'on:\n  pull_request:\n  pull_request_target:\njobs: {}')
expect_dual fail "a workflow on both events (map form)" "$d"

d=$(dual_case dual-list 'on: [pull_request, pull_request_target]')
expect_dual fail "a workflow on both events (list form)" "$d"

d=$(dual_case target-only $'# a pull_request run lives elsewhere\non:\n  pull_request_target:\njobs:\n  x:\n    if: github.event.pull_request.number > 0')
expect_dual pass "pull_request_target only, pull_request named in a comment and a context" "$d"

d=$(dual_case target-step $'on:\n  pull_request_target:\njobs:\n  x:\n    steps:\n      - run: echo \'pull_request\'')
expect_dual pass "pull_request_target only, a step that prints pull_request" "$d"

d=$(dual_case dual-quoted $'"on":\n  pull_request:\n  pull_request_target:')
expect_dual fail "a workflow on both events (quoted on key)" "$d"

d=$(dual_case pr-review $'on:\n  pull_request:\n  pull_request_review:')
expect_dual pass "pull_request with pull_request_review" "$d"

if [[ $fails -ne 0 ]]; then
  echo "guard_test: ${fails} case(s) failed" >&2
  exit 1
fi
echo "guard_test: all cases passed"
