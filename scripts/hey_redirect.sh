#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-50000}"
concurrency="${CONCURRENCY:-300}"
hey_bin="${HEY_BIN:-./bin/hey}"

body="$(curl -fsS -X POST "${base_url}/api/links" \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/flashlink-hey-read"}')"
code="$(printf '%s' "${body}" | sed -n 's/.*"code":"\([^"]*\)".*/\1/p')"
if [[ -z "${code}" ]]; then
  echo "could not parse code from response: ${body}" >&2
  exit 1
fi

echo "target=${base_url}/${code} requests=${requests} concurrency=${concurrency}"
"${hey_bin}" -n "${requests}" -c "${concurrency}" -disable-redirects "${base_url}/${code}"
