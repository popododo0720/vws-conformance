# vws-conformance

Unmodified AWS SDK conformance suite for VWS. Expected results come from the AWS docs. The suite never calls `amazonaws.com`.

There is no nightly job and no self-hosted runner. Pull requests run the fake console-api job. A vws-test run is manual.

## Environment

| Variable | Meaning |
| --- | --- |
| `VWS_CONF_ACCESS_KEY_ID` | Access key. The fake backend uses `AKIDEXAMPLE`. |
| `VWS_CONF_SECRET_ACCESS_KEY` | Secret key. Never print it. |
| `VWS_CONF_SESSION_TOKEN` | Optional session token. |
| `VWS_CONF_ACCOUNT_ID` | 12 digit account. The fake backend uses `123456789012`. |
| `VWS_CONF_EC2_ENDPOINT` | EC2, VPC, and EBS listener. |
| `VWS_CONF_S3_ENDPOINT` | S3 listener. |
| `VWS_CONF_IAM_ENDPOINT` | IAM and STS listener. |
| `VWS_CONF_ALLOWED_HOSTS` | Comma-separated hosts. AWS endpoint names are refused even if listed. A public IP is allowed only when that IP is listed. Link-local addresses are refused. |
| `VWS_CONF_ALLOWED_ACCOUNTS` | Comma-separated account ids. |
| `VWS_CONF_TARGET` | `local-fake`, `local-grpc`, or `vws-test`. |
| `VWS_CONF_RUN_ID` | Optional. Empty mints `conf-YYYYMMDDThhmmssZ-xxxxxx`. A set value must match that pattern. |
| `VWS_CONF_OUT` | Report directory. Default is `out/` at the module root. |
| `VWS_CONF_PARALLEL` | Case concurrency. Default is 1. |
| `VWS_CONF_CONSOLE_API_VERSION` | Recorded in the report. |
| `VWS_CONF_FULL` | `1` runs every group. |
| `VWS_CONF_SERVICE` | Same as `--service`. |
| `VWS_CONF_GROUP` | Same as `--group`. |

`go test ./...` does not run live groups. Pass `-args -service s3 -group buckets`, or `-args -full`.

```
go run ./cmd/conf-run --service s3 --group buckets
```

Instance launches use `t2.nano`. Volumes are 1 GiB.

`scripts/cli/s3.sh` runs from the module root. It refuses `AWS_PROFILE` and calls `go run ./cmd/conf-run check-endpoint` before `aws`. The check resolves each configured endpoint and does not call the service. The script then exports the `VWS_CONF_*` keys and points the AWS CLI config files at `/dev/null`.

## Local fake

Build console-api with `-backend fake -iam fake`. Then:

```
export VWS_CONF_ACCESS_KEY_ID=AKIDEXAMPLE
export VWS_CONF_SECRET_ACCESS_KEY=secret
export VWS_CONF_ACCOUNT_ID=123456789012
export VWS_CONF_EC2_ENDPOINT=http://127.0.0.1:8081
export VWS_CONF_S3_ENDPOINT=http://127.0.0.1:8082
export VWS_CONF_IAM_ENDPOINT=http://127.0.0.1:8084
export VWS_CONF_ALLOWED_HOSTS=127.0.0.1
export VWS_CONF_ALLOWED_ACCOUNTS=123456789012
export VWS_CONF_TARGET=local-fake
export VWS_CONF_CONSOLE_API_VERSION=local
go test -p 1 -count=1 -timeout 300s ./s3 ./ec2 ./vpc ./ebs ./iam ./sts ./auth -args -full
```

## Manual vws-test

Load the env file with `set -a` and `set +x`. Do not echo the secret. Point the three endpoints at the vws-test listeners, put that host in `VWS_CONF_ALLOWED_HOSTS`, and set `VWS_CONF_TARGET=vws-test`. Then run the same `go test` command with `-args -full`. IAM and STS cases stay skipped until vws-deploy#310 lands.

## Add a case

Add a `cases.Case` to the service group and a branch in that group's runner. The doc URL is an `https://docs.aws.amazon.com/` page. If the operation is new, add a row to `inventory/inventory.yaml`. Run `go test ./internal/inventory/` and the one group.

## Triage a failure

`status=fail` means VWS differed from the AWS doc. Do not edit the expected code. Run:

```
go run ./cmd/conf-issues out/<run-id>/<service>-<group>.json
```

The text is a dry-run issue. dr eggbot files it. `status=error` is a harness bug and fails the Go test.

## Cleanup

Names and tags carry the run id. `cmd/conf-run` runs cleanup on success, failure, panic, and SIGINT/SIGTERM. The next run sweeps leftover `conf-` resources and nothing else.

```
go run ./cmd/conf-sweep --dry-run
```
