#!/bin/sh
# All-in-one demo entrypoint: runs api + scheduler + two workers in a single
# container for free-tier hosts (Render/SnapDeploy) that provide one process
# per service. Leases and fencing tokens are coordinated through Postgres, so
# the multi-process failure proofs behave identically to separate services.
set -eu

: "${DURABLEGO_DATABASE_URL:?DURABLEGO_DATABASE_URL is required}"

/durablego-api &
API_PID=$!
/durablego-scheduler &
SCHEDULER_PID=$!
DURABLEGO_WORKER_ID="${DURABLEGO_WORKER_A_ID:-worker-a}" /durablego-worker &
WORKER_A_PID=$!
DURABLEGO_WORKER_ID="${DURABLEGO_WORKER_B_ID:-worker-b}" /durablego-worker &
WORKER_B_PID=$!

# Self-driving demo data: continuous realistic workflow submissions so the
# dashboard always shows live executions completing.
DEMO_DRIVER_PID=""
if [ "${DEMO_DRIVER:-true}" = "true" ]; then
  DURABLEGO_API_URL="${DURABLEGO_API_URL:-http://127.0.0.1:${PORT:-8080}}" /durablego-demo-driver &
  DEMO_DRIVER_PID=$!
fi

# Portable `wait -n` replacement: busybox ash does not support wait -n, so poll
# the pids directly and exit (letting the platform restart us) when any
# component dies.
PIDS="$API_PID $SCHEDULER_PID $WORKER_A_PID $WORKER_B_PID"
if [ -n "$DEMO_DRIVER_PID" ]; then
  PIDS="$PIDS $DEMO_DRIVER_PID"
fi

while sleep 2; do
  for pid in $PIDS; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "allinone: component pid $pid exited, shutting down for restart" >&2
      exit 1
    fi
  done
done
