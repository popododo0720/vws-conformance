# External pve-test read run

The verification box runs `sh scripts/pve-test.sh` from a reviewed checkout,
nightly through its external timer or manually through the same runner.
No GitHub Actions workflow gets box credentials. Timer integration and
report archival are tracked in
https://github.com/popododo0720/vws-deploy/issues/831.
Live execution is deferred until after phase 1, when the nested box exists.

The runner supplies these variables from its environment and systemd
credentials. Do not use AWS credentials or profiles, or a PVE token: this
suite authenticates to the nested box's VWS endpoints with throwaway VWS
credentials.

| Variable | Runner value |
| --- | --- |
| `VWS_CONF_ALLOWED_HOSTS` | Exact host of the nested box VWS endpoints |
| `VWS_CONF_EC2_ENDPOINT` | Nested box VWS EC2 endpoint URL |
| `VWS_CONF_IAM_ENDPOINT` | Nested box VWS IAM/STS endpoint URL |
| `VWS_CONF_REGION` | `pve-test-1` |
| `VWS_CONF_ACCOUNT_ID` | Throwaway VWS account allowed for this run |
| `VWS_CONF_ACCESS_KEY_ID` | Throwaway VWS key from the credential store |
| `VWS_CONF_SECRET_ACCESS_KEY` | Throwaway VWS secret from the credential store |
| `VWS_CONF_SESSION_TOKEN` | Optional VWS session credential |
| `VWS_CONF_CONSOLE_API_VERSION` | Deployed console-api revision for report metadata |
| `VWS_CONF_OUT` | Persistent report directory archived by the external runner |

The entrypoint forces target `pve-test`, the matching account allowlist and
`-full -read-only`. The engine also filters `pve-test` independently of the
flag, so a direct `conf-run -full` cannot admit creating fixtures. It selects
29 cases across EC2, EBS, VPC and AUTH; skipped cases retain their skip records.
Describe cases that create fixtures are excluded. The existing endpoint,
account and guarded dialer checks remain in force.

Preview the exact groups without credentials or endpoint traffic:

```sh
go run ./cmd/conf-select -target pve-test
```

The run returns nonzero for unlisted parity failures and harness errors.
`internal/engine/run_test.go` (`TestParityGate`) proves that a failure without
an issue URL fails the parity gate; `internal/engine/known_test.go` proves
malformed or unknown case mappings fail closed. Reports remain under
`VWS_CONF_OUT/<run-id>/` on failure too. Archive those reports even on nonzero
exit, and propagate that status from the timer. A fresh process mints a new
run ID, so a rerun does not overwrite the previous report.

## Known gaps without current case coverage

- VM-wide snapshot scope:
  https://github.com/popododo0720/vws-console-api/issues/1002.
  `ebs/snapshots.go` checks lifecycle and source identity, with no multi-volume
  isolation assertion. Its lifecycle IDs cannot represent this limitation.
- CreateVolume from SnapshotId unsupported:
  https://github.com/popododo0720/vws-console-api/issues/1003.
  No case in `ebs/volumes.go` sets SnapshotId.
- No terminated state after delete: the TERM cases and their target-scoped
  known-issue line belong to the sibling work for conformance #139. This
  branch stays based on main.

No failure mappings are invented for these missing cases. AC1 of #140 remains
incomplete until the cases and corresponding scoped records exist.
