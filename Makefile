.PHONY: test race fmt run-api run-scheduler run-worker compose-config

fmt:
	go fmt ./...

test:
	go test ./...

race:
	go test -race ./...

run-api:
	go run ./cmd/api

run-scheduler:
	go run ./cmd/scheduler

run-worker:
	go run ./cmd/worker

compose-config:
	docker compose config

