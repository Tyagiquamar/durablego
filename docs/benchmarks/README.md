# Benchmark Reports

Run:

```bash
go run ./cmd/loadgen -workflows 1000 -workers 1
go run ./cmd/loadgen -workflows 1000 -workers 2
go run ./cmd/loadgen -workflows 1000 -workers 4
go run ./cmd/loadgen -workflows 1000 -workers 8
```

Record workflows/sec, activities/sec, p50/p95/p99 latency, retry rate, queue depth, and database pressure for each run once the PostgreSQL repository is wired in.

