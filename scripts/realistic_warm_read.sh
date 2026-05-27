#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-10000}"
concurrency="${CONCURRENCY:-100}"
codes_file="${CODES_FILE:-tmp/flashlink_codes.txt}"

if [[ ! -f "${codes_file}" ]]; then
  CODES="${CODES:-10000}" CODES_FILE="${codes_file}" bash scripts/prepare_codes.sh
fi

go build -o bin/loadtest ./cmd/loadtest
bin/loadtest \
  -base "${base_url}" \
  -codes "${codes_file}" \
  -n "${requests}" \
  -c "${concurrency}" \
  -mode random \
  -expected 302
