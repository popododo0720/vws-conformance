# Harness

The suite is a table of cases. Each case names one AWS operation, one expected HTTP status, and one expected error code taken from the AWS docs. `internal/expect.Judge` compares the captured response. A mismatch is status `fail`. A transport or harness mistake is status `error` and fails the Go test.

Clients are unmodified aws-sdk-go-v2. The only overrides are the base endpoint, region `vws-1`, and S3 path style. Capture middleware records the request and response. It does not change them.

Every run id matches `conf-YYYYMMDDThhmmssZ-xxxxxx`. Resource names and the `vws:conformance-run` tag use that id. Cleanup is registered LIFO, `TestMain` runs it on the way out, and SIGINT/SIGTERM run the same function. A panic in a case is recovered and still recorded. The process cleanup also runs.

Before mutations, `CheckEndpoints` refuses `amazonaws.com` and any host that is not on the allow list. `GetCallerIdentity` then checks the account. The pre-run sweep deletes only resources that carry the `conf-` prefix.

`go test` without `-service`, `-group`, or `-full` skips live groups. `cmd/conf-run --service s3 --group buckets` runs one group. A pull request runs unit tests plus the groups `internal/touch` selects from the changed paths. A change under `internal/`, `schema/`, `cmd/`, the module files, `scripts/`, or the workflow is a full run. `workflow_dispatch` is a full run. There is no schedule.
