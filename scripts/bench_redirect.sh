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

results="$(
  seq "${requests}" | xargs -I{} -P "${concurrency}" curl -s -o /dev/null -w '%{http_code}\n' "${base_url}/${code}" || true
)"

end_ns="$(date +%s%N)"
elapsed_ms="$(( (end_ns - start_ns) / 1000000 ))"
if [[ "${elapsed_ms}" -eq 0 ]]; then
  elapsed_ms=1
fi

qps="$(( requests * 1000 / elapsed_ms ))"
success="$(printf '%s\n' "${results}" | awk '$1 == 302 { ok++ } END { print ok + 0 }')"
failed="$(( requests - success ))"
echo "elapsed_ms=${elapsed_ms} approx_qps=${qps}"
echo "success=${success} failed=${failed} expected_status=302"
