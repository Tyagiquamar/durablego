#!/bin/sh
# All-in-one demo entrypoint: runs api + scheduler + two workers in a single
# container for free-tier hosts (Render/SnapDeploy) that provide one process
# per service. Leases and fencing tokens are coordinated through Postgres, so
# the multi-process failure proofs behave identically to separate services.
set -eu

: "${DURABLEGO_DATABASE_URL:?DURABLEGO_DATABASE_URL is required}"

/durablego-api &
/durablego-scheduler &
DURABLEGO_WORKER_ID="${DURABLEGO_WORKER_A_ID:-worker-a}" /durablego-worker &
DURABLEGO_WORKER_ID="${DURABLEGO_WORKER_B_ID:-worker-b}" /durablego-worker &

# Exit (and let the platform restart the container) when any component dies.
wait -n
echo "allinone: a component exited, shutting down for restart" >&2
exit 1
