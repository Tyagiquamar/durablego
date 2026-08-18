# DurableGo

DurableGo is a portfolio-first durable workflow execution engine for studying reliable execution across process crashes, retries, duplicate delivery, stale workers, and scheduler restarts.

It is intentionally not a Temporal clone. The v1 product proves a smaller set of distributed-systems guarantees with inspectable state, event history, worker leases, fencing tokens, and reproducible failure scenes.

## Guarantees

- Workflow state is durable in PostgreSQL in the production storage contract.
- Activity execution is at least once, not exactly once.
- Starting a workflow with the same namespace and idempotency key returns the original execution.
- Claimed activities carry a lease owner and fencing token.
- Stale heartbeats, completions, and failures are rejected after another worker reclaims an activity.
- External side effects are not exactly once; applications must provide idempotency keys for side-effecting activities.

## Current Proof Surface

- In-memory correctness engine with the same state and fencing rules as the PostgreSQL schema.
- SQL migration for namespaces, definitions, workflow executions, activity executions, dependencies, idempotency keys, and ordered event history.
- REST API for workflow start, list, inspect, cancel, worker poll, heartbeat, complete, fail, health, readiness, and Prometheus-style metrics.
- Scheduler pass for retry readiness and expired lease recovery.
- Worker runtime with bounded concurrency and handler registration.
- Failure tests for duplicate start, concurrent claim safety, stale completion rejection, dependency readiness, and retry exhaustion.
- Dashboard shell for portfolio proof scenes.

## Run Locally

```bash
docker compose up
```

API:

```bash
curl -X POST http://localhost:8080/v1/workflows \
  -H "content-type: application/json" \
  -d '{"namespace":"production","name":"order-processing","idempotency_key":"order-129331","activities":[{"name":"validate","task_queue":"default"},{"name":"payment","task_queue":"default","depends_on":["validate"]}]}'
```

Windows helper:

```powershell
.\scripts\start-order-workflow.ps1
```

Tests:

```bash
go test ./...
go test -race ./...
```

If Go is not installed locally, run:

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.23 go test ./...
```

## Proof Scenes

### Stale Worker Fencing

1. Worker A claims an activity and receives fencing token 1.
2. Worker A freezes past its lease.
3. The scheduler marks the activity ready again.
4. Worker B claims the same activity with fencing token 2.
5. Worker A resumes and tries to complete with token 1.

Invariant: Worker A's stale completion is rejected and cannot overwrite Worker B's outcome.

### Duplicate Workflow Start

1. A client starts a workflow with namespace `production` and idempotency key `order-129331`.
2. The client repeats the same request after a simulated network timeout.

Invariant: the API returns the original workflow execution instead of creating a duplicate.

### Retry Exhaustion

1. A worker claims an activity.
2. The handler reports retryable failure until attempts are exhausted.

Invariant: activity and workflow move to failed state and event history records the terminal transition.

## Roadmap

- Wire the execution services to PostgreSQL repositories.
- Add gRPC/protobuf worker transport in parallel with the REST worker API.
- Expand DAG execution demos with parallel payment and inventory branches.
- Add OpenTelemetry spans and a local Prometheus/Grafana stack.
- Record benchmark artifacts for 1, 2, 4, and 8 worker runs.
- Add hosted demo deployment with API-key protection for mutating endpoints.
