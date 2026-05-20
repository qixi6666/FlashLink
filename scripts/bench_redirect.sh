#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-1000}"
concurrency="${CONCURRENCY:-50}"

body="$(curl -fsS -X POST "${base_url}/api/links" \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/flashlink-bench"}')"
code="$(printf '%s' "${body}" | sed -n 's/.*"code":"\([^"]*\)".*/\1/p')"
if [[ -z "${code}" ]]; then
  echo "could not parse code from response: ${body}" >&2
  exit 1
fi

echo "target=${base_url}/${code} requests=${requests} concurrency=${concurrency}"
start_ns="$(date +%s%N)"

seq "${requests}" | xargs -I{} -P "${concurrency}" curl -fsS -o /dev/null -L "${base_url}/${code}"

end_ns="$(date +%s%N)"
elapsed_ms="$(( (end_ns - start_ns) / 1000000 ))"
if [[ "${elapsed_ms}" -eq 0 ]]; then
  elapsed_ms=1
fi

qps="$(( requests * 1000 / elapsed_ms ))"
echo "elapsed_ms=${elapsed_ms} approx_qps=${qps}"

