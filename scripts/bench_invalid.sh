#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-1000}"
concurrency="${CONCURRENCY:-50}"

echo "target=${base_url}/invalid-code requests=${requests} concurrency=${concurrency}"
start_ns="$(date +%s%N)"

results="$(
  seq "${requests}" | xargs -I{} -P "${concurrency}" curl -s -o /dev/null -w '%{http_code}\n' "${base_url}/invalid-code-{}" || true
)"

end_ns="$(date +%s%N)"
elapsed_ms="$(( (end_ns - start_ns) / 1000000 ))"
if [[ "${elapsed_ms}" -eq 0 ]]; then
  elapsed_ms=1
fi

qps="$(( requests * 1000 / elapsed_ms ))"
success="$(printf '%s\n' "${results}" | awk '$1 == 404 { ok++ } END { print ok + 0 }')"
failed="$(( requests - success ))"
echo "elapsed_ms=${elapsed_ms} approx_qps=${qps}"
echo "success=${success} failed=${failed} expected_status=404"
