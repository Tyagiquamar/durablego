.PHONY: fmt vet test test-unit test-pg test-failure verify run-api run-scheduler run-worker compose-config

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test -count=1 ./...

# Note: -race needs CGO_ENABLED=1 + gcc; use `make race` where available.
test-unit:
	go test -short ./...

test-pg:
	go test -count=1 ./tests/postgres/ ./internal/persistence/ ./tests/integration/

test-failure:
	go test -count=1 -v -run TestWorkerProcessKillRecovery ./tests/failure/

race:
	CGO_ENABLED=1 go test -race ./...

verify: vet test-unit test-pg test-failure

run-api:
	go run ./cmd/api

run-scheduler:
	go run ./cmd/scheduler

run-worker:
	go run ./cmd/worker

compose-config:
	docker compose config
