# Agent rules

This repository is the unmodified AWS SDK and CLI conformance suite for VWS. It talks to VWS only through the AWS SDK or CLI. It does not import any other `github.com/popododo0720/vws-*` module.

## Rules

| ID | Rule | Check |
| --- | --- | --- |
| R1 | Work on a branch. Never push to main or master. | `.githooks/pre-push` |
| R2 | Commit subject is `type: summary`. The body contains `Part of #N`. The commit message has no `Closes`, `Fixes`, or `Resolves`. | review |
| R3 | The pull request body starts with `Made-by: Cursor cloud agent` when a cloud agent opened it. The body contains `Closes #N` for the tracking issue this change finishes. | review |
| R7 | No logo files and no trademark notices. | review |
| R9 | A cloud agent may open a pull request. The first body line is `Made-by: Cursor cloud agent`. | review |
| R13 | English only in code, comments, docs, commits, and the pull request body. | `scripts/guard.sh` |
| R21 | `go.mod` does not require another vws module and does not replace aws-sdk-go-v2. | `scripts/guard.sh` |
| R22 | The endpoint allow list refuses `amazonaws.com` and `*.amazonaws.com`. The suite has no real-AWS mode. | `internal/harness/allow_test.go` |
| R23 | Workflows have `pull_request` and `workflow_dispatch` only. No `schedule`. No self-hosted runner. | `scripts/guard.sh` |
| R24 | A parity mismatch stays a recorded fail. Do not change the expected AWS code to match VWS. | `internal/expect/expect_test.go` |

`make guard` runs `scripts/guard.sh`. `make test-unit` runs the race tests that do not need a backend. `make test-local` runs every group against a fake console-api that is already listening.

## Way of working

Write the smallest change that keeps the case table honest. A new AWS expectation is a new case, not a change to an old expected code. Say a command was verified only if it was run. Mark everything else unverified.

## Insider-risk guardrails

1. The author of a change does not merge it. dr eggbot owns merges.
2. Commits use `Part of #N` and do not use a closing keyword. The pull request body may contain `Closes #N`.
3. New behavior has a failure-path test, or the pull request says why that is impossible.
4. No direct push to main. Tokens stay scoped to this repo. Secrets stay in env and out of `out/`.
5. The author does not approve their own verify-gate comment.
6. Stop when the user says to stop.
7. A rule break is recorded, not hidden.

## Verify gate

vws-verify judges the nine QUALITY.md items and posts `Verified-by` with a Codex review path. This file does not claim that the gate has passed.
