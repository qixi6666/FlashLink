#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-10000}"
concurrency="${CONCURRENCY:-100}"
codes_file="${CODES_FILE:-tmp/flashlink_codes.txt}"
reset_cache="${RESET_CACHE:-false}"

if [[ ! -f "${codes_file}" ]]; then
  CODES="${CODES:-10000}" CODES_FILE="${codes_file}" bash scripts/prepare_codes.sh
fi

if [[ "${reset_cache}" == "true" ]]; then
  echo "flushing Redis and restarting gateway so local cache is empty and Redis filter is rebuilt"
  docker compose exec -T redis redis-cli FLUSHDB >/dev/null
  docker compose restart gateway >/dev/null
  ready="false"
  for _ in $(seq 1 30); do
    if curl -fsS "${base_url}/healthz" >/dev/null 2>&1; then
      ready="true"
      break
    fi
    sleep 1
  done
  if [[ "${ready}" != "true" ]]; then
    echo "gateway did not become healthy after restart" >&2
    exit 1
  fi
fi

go build -o bin/loadtest ./cmd/loadtest
bin/loadtest \
  -base "${base_url}" \
  -codes "${codes_file}" \
  -n "${requests}" \
  -c "${concurrency}" \
  -mode sequential \
  -expected 302
