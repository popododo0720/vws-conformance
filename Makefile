export PATH := $(HOME)/sdk/go/bin:$(PATH)
export GOTOOLCHAIN := local

.PHONY: test test-unit test-local sweep guard fmt

test: test-unit

test-unit:
	go test -race -count=1 -p 1 ./internal/...

test-local:
	go test -p 1 -count=1 -timeout 300s ./s3 ./ec2 ./vpc ./ebs ./iam ./sts ./auth -args -full

sweep:
	go run ./cmd/conf-sweep --dry-run

guard:
	sh scripts/guard.sh

fmt:
	gofmt -w .
