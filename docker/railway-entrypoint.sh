#!/bin/sh
set -eu

case "${DURABLEGO_RUN:-api}" in
  api)
    exec /durablego-api
    ;;
  scheduler)
    exec /durablego-scheduler
    ;;
  worker)
    exec /durablego-worker
    ;;
  *)
    echo "invalid DURABLEGO_RUN: ${DURABLEGO_RUN}" >&2
    exit 64
    ;;
esac