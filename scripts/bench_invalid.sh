#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
requests="${REQUESTS:-1000}"
concurrency="${CONCURRENCY:-50}"

echo "target=${base_url}/invalid-code requests=${requests} concurrency=${concurrency}"
start_ns="$(date +%s%N)"

seq "${requests}" | xargs -I{} -P "${concurrency}" curl -fsS -o /dev/null "${base_url}/invalid-code-{}" || true

end_ns="$(date +%s%N)"
elapsed_ms="$(( (end_ns - start_ns) / 1000000 ))"
if [[ "${elapsed_ms}" -eq 0 ]]; then
  elapsed_ms=1
fi

qps="$(( requests * 1000 / elapsed_ms ))"
echo "elapsed_ms=${elapsed_ms} approx_qps=${qps}"

