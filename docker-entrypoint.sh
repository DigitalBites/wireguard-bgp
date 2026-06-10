#!/bin/sh
set -eu

SUPERVISOR_SOCKET="${SUPERVISOR_SOCKET:-/run/peplink-wg-bgp/supervisor.sock}"
export SUPERVISOR_SOCKET

cleanup() {
  if [ -n "${SUPERVISOR_PID:-}" ]; then
    kill "$SUPERVISOR_PID" 2>/dev/null || true
    wait "$SUPERVISOR_PID" 2>/dev/null || true
  fi
}
trap cleanup INT TERM EXIT

/usr/local/bin/peplink-wg-bgp supervisor &
SUPERVISOR_PID="$!"

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if [ -S "$SUPERVISOR_SOCKET" ]; then
    break
  fi
  sleep 0.1
done

exec su-exec app /usr/local/bin/peplink-wg-bgp serve
