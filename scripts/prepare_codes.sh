#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"
count="${CODES:-10000}"
concurrency="${CONCURRENCY:-50}"
output="${CODES_FILE:-tmp/flashlink_codes.txt}"
force="${FORCE:-false}"

if [[ "${force}" != "true" && -f "${output}" ]]; then
  existing="$(wc -l < "${output}")"
  if [[ "${existing}" -ge "${count}" ]]; then
    echo "using existing codes file: ${output} lines=${existing}"
    exit 0
  fi
fi

mkdir -p "$(dirname "${output}")"
tmp_file="${output}.tmp"
echo "creating ${count} short links into ${output} concurrency=${concurrency}"

seq "${count}" | xargs -I{} -P "${concurrency}" sh -c '
  body="$(curl -fsS -X POST "$0/api/links" \
    -H "Content-Type: application/json" \
    -d "{\"long_url\":\"https://example.com/realistic/$1\"}")"
  code="$(printf "%s" "${body}" | sed -n "s/.*\"code\":\"\\([^\"]*\\)\".*/\\1/p")"
  if [ -z "${code}" ]; then
    echo "could not parse code from response: ${body}" >&2
    exit 1
  fi
  printf "%s\n" "${code}"
' "${base_url}" {} > "${tmp_file}"

mv "${tmp_file}" "${output}"
echo "codes_file=${output} lines=$(wc -l < "${output}")"
