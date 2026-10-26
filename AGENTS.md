# AGENTS.md

vws-conformance is the AWS conformance suite for VWS. It drives VWS endpoints with the unmodified AWS SDK and CLI and checks that every response matches what the official AWS documentation says AWS returns.

## Hard safety rules (read first)

These rules have no exceptions. Breaking one is an incident (see guardrail 7).

1. **Never send any request to real AWS.** No code path, test, script, CI job or manual run may reach an `amazonaws.com` (or any other AWS-owned) endpoint, including STS, IMDS and SSO.
2. **VWS endpoints only, through the guard.** The suite talks only to VWS endpoints, and only through the guarded dialer and the host allowlist (`VWS_CONF_ALLOWED_HOSTS`). Never bypass, widen or disable the guard, never follow redirects off the allowlist, and never let a proxy or a fresh DNS lookup pick the destination. The AWS CLI runs only behind the same endpoint check.
3. **No real AWS credentials or profiles, ever.** Use only the throwaway VWS test credentials from the environment. Never read `~/.aws`, `AWS_PROFILE`, SSO caches or instance metadata, and never commit or log a credential.
4. **Parity is checked against official AWS docs only.** Every expected status, error code and message cites the official AWS documentation (API reference or user guide). Never derive an expectation by calling real AWS, and never loosen one to make a VWS response pass.
5. **Cleanup touches only the suite's own resources.** A run deletes only resources named with its own `conf-` run ID.

## VWS rules

- **Exact AWS parity is the top priority.** Every gap between VWS and AWS is a tracked issue in the repo that owns the behavior. A failing case is never hidden, skipped or loosened without a linked issue.
- **Codex review once per head.** Every pull request head gets one Codex review; a new head needs a new review.
- **verify PASS before merge.** vws-verify must post PASS (`Verified-by: vws-verify` for the exact head SHA, `verify-approved` label) and `verify-gate` must be green before merge.
- **Only dr eggbot merges.** No author bot merges, including its own pull request.
- **English only.** See "English only" below.
- Pull requests say `Part of #N` and never close an issue (no Closes/Fixes/Resolves in the body or commits). No force-push.

## Branches and commits

- Do not push to main directly. Work on a branch cut from main, one pull request per branch.
- Commits are atomic. One commit is one change, and the build holds with that commit alone.

## Rules

A rule whose enforcer cell is `Judgment` has no check; the author and reviewer uphold it.

| Rule | Enforcer |
| --- | --- |
| Never send any request to real AWS | Judgment until the harness guard tests land (vws-conformance#1) |
| Requests go only to allowlisted VWS endpoints through the guarded dialer | Judgment until the harness guard tests land (vws-conformance#1) |
| No real AWS credentials or profiles | Judgment |
| Expected results cite official AWS docs only | Judgment |
| Every VWS-vs-AWS gap is a tracked issue | Judgment |
| Do not push to main directly. Work on a branch | `.github/workflows/direct-push-watch.yml` (detected after the fact) |
| A pull request targets main, says `Part of #N` and closes no issue | `.github/workflows/repo-guard.yml` `pr-hygiene` |
| Merge only after vws-verify PASS for the exact head | `.github/workflows/repo-guard.yml` `verify-gate` |
| Commits are atomic | Judgment |

## English only

Write everything in this repo in English: README, code comments, docs, AGENTS.md, QUALITY.md, PR and issue templates, commit messages, PR and issue bodies, review comments and `Verified-by` sections. The `english` job in CI runs `scripts/check-english.sh`: `files` fails on a tracked file with Hangul unless `.github/english-allowlist` lists it as `<glob> <reason>` (only test data and user-facing UI strings belong there), and `text` and `commits` reject Hangul in the PR body and the PR's commit messages with no exception.

## /correct loop

1. When a bot is corrected (by the user Su Kim, tmp or eggbot, or by a failed `vws verify`), it opens an issue the same day with the `incident` label using the `.github/ISSUE_TEMPLATE/incident.yml` form. It must fill in the correction and "the check that would have caught this".
2. Every few days vws-verify runs a gardener pass. It groups open incidents by cause and promotes repeated ones to a check (test, lint, guard rule, archtest), not a sentence. The check must actually fail on an input that contains the mistake.
3. When the promoting PR is merged, close the incident with its link. An incident that will not be promoted is closed with the reason.
4. The milestone report (to tmp) lists the number of incidents in the period and the checks promoted.
5. The `incident` label is defined in `.github/labels.yml`, copied from vws-deploy. That file is only a definition and does not create the label on GitHub. `scripts/guard.sh` runs `scripts/guard-agents.sh correct`, which checks this section and its body, the required fields of the form and the label definition; the conformance workflow runs both on every pull request and `scripts/guard_test.sh` shows it fails on an empty section.

## Insider-risk guardrails

Bots are insider risks too. The rules below have no exceptions. The design is popododo0720/vws-deploy#33.

1. **Independent verification**: an important result (merge, security, data) is never verified by the bot/model that wrote it. Another bot (or a person) verifies it. This is a process rule; CI cannot enforce it.
2. **Records**: every change leaves a trail of issue → PR (`Part of #N`) → commits. Work logs go in the PR body and issue comments in a form people can read. Force-push and history rewriting are forbidden, except with the user's approval.
3. **Failure tests**: a new feature is not tested on the happy path only. Include at least one case each for power loss and restart, idempotent re-run, and malicious or boundary input, or state in the PR why that is not possible.
4. **Least privilege**: do not push to main directly. Every change is a single PR from its own branch cut from main. Stacked PRs and local merges are forbidden. Tokens are scoped to the repo. RBAC allows only the resources the service code actually calls, as a namespace-scoped Role where possible.
5. **Separated audit**: the author bot, the verifier bot and the merge decision are all different. An author bot never merges its own PR. Merge only when the base is main and every CI check on the head commit is success.
6. **Isolation and emergency stop**: a bot is stopped immediately at the user's request or by the orchestrator stopping that bot's session and revoking its token (`gh auth`, PAT revoke). Risky work happens only in a sandbox (the box, a throwaway cluster). No access to production clusters.
7. **Incident disclosure**: rule violations and incidents are never hidden; record them in an issue as `Cause / Impact / Action / Prevention`. The label is `incident`.

This repo is public. `main` branch protection requires the CI checks; `.github/workflows/repo-guard.yml` checks that the PR's base is main, that the body has `Part of #N` and no closing keyword, and that this section has items 1-7. `.github/workflows/direct-push-watch.yml` reports after the fact any commit that reached main without a PR. The check code is `scripts/guard-agents.sh`, the same file as in vws-deploy, so it is copied, not edited.

### Script push, PR and merge

A deterministic script (the factory orchestrator in vws-deploy `docs/factory-orchestrator.md`, the handoff submit) may push a branch, open a pull request and merge one. A script merge happens only when all of these hold:

- every CI check on the head commit is green;
- `verify-gate` passes;
- the `vws-verify` App check run passes;
- the pull request has no conflicts;
- the merge is a squash, made with `--match-head-commit` set to the verified head;
- the pull request is not labeled `blocked`.

`ORCHESTRATE_MERGE=apply` is approved only under these conditions. The lead (dr eggbot) is woken only for a decision or a failure (issue scope, real conflicts, a failed check), not for a routine push, pull request or merge. Guardrails 1, 4 and 2 still hold: the author bot is never the verifier, nothing pushes to main, and every pull request body has `Part of #N` with no closing keyword.

