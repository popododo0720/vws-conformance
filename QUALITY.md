# Quality standard

Adopted from vws-deploy on 2026-10-15. Every change in this repo must be AWS-grade production quality, not demo quality. A PR is done only when the "Quality checklist (required)" in the pull request template is filled with evidence for each item.

1. **Error handling**: every error is wrapped with context and returned. No panic or crash on any input.
2. **Retries and timeouts**: every external call has a timeout or context deadline and retries transient failures with bounded backoff.
3. **Security**: validate all input, grant least privilege, and keep secrets out of logs and out of the repo.
4. **Unit tests** for new behavior and failure paths.
5. **Integration tests** at real or faithfully faked boundaries.
6. **Live evidence on vws-test**: commands and output from the vws-test cluster, linked or pathed in the PR. Without live evidence the work is not done. This repository has no nightly job and no self-hosted runner. The vws-test run is the manual command in the README.
7. **Observability**: OpenTelemetry traces and metrics plus structured logs. A library or a test suite with no long-running process records results in the conformance report instead.
8. **Idempotency**: safe on retry, duplicate request and restart.
9. **AWS compatibility** where applicable: API behavior and error codes match AWS. A parity failure is recorded. It is not fixed by changing the expected code.

Reviewers do not merge a PR whose checklist lacks evidence or a stated reason for N/A.

## Verify gate

The verify gate is the same process as vws-deploy. vws-verify posts the `Verified-by` comment and attaches the Codex review. This repository does not claim that gate on its own pull requests. See `docs/QUALITY.md` in vws-deploy for the comment shape and the known gaps.
