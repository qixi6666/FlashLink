#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-50000}"
concurrency="${CONCURRENCY:-300}"
hey_bin="${HEY_BIN:-./bin/hey}"

echo "target=${base_url}/api/links requests=${requests} concurrency=${concurrency}"
"${hey_bin}" -n "${requests}" -c "${concurrency}" \
  -m POST \
  -T application/json \
  -d '{"long_url":"https://example.com/flashlink-hey-create"}' \
  "${base_url}/api/links"
