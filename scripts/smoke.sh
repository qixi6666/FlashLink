#!/usr/bin/env bash
set -euo pipefail

base_url="${BASE_URL:-http://127.0.0.1:8080}"

echo "checking health"
curl -fsS "${base_url}/healthz"
echo

echo "creating short link"
body="$(curl -fsS -X POST "${base_url}/api/links" \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/flashlink"}')"
echo "${body}"

code="$(printf '%s' "${body}" | sed -n 's/.*"code":"\([^"]*\)".*/\1/p')"
if [[ -z "${code}" ]]; then
  echo "could not parse code from response" >&2
  exit 1
fi

echo "checking redirect headers for code=${code}"
curl -fsSI "${base_url}/${code}" | sed -n '1,6p'

echo "checking stats"
curl -fsS "${base_url}/api/links/${code}/stats"
echo

