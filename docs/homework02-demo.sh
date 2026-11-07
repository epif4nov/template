#!/usr/bin/env bash
# Run against an already running service. Requires curl and jq.
set -euo pipefail
base=${BASE_URL:-http://localhost:8080}
driver=$(cat /proc/sys/kernel/random/uuid)
user=$(cat /proc/sys/kernel/random/uuid)
missing=$(cat /proc/sys/kernel/random/uuid)
payload=$(jq -nc --arg user "$user" --arg driver "$driver" '{user_id:$user,driver_id:$driver,start_point:{latitude:59.9398,longitude:30.3146},end_point:{latitude:59.929,longitude:30.3626},price:1450}')
work=$(mktemp -d)
trap 'rm -f "$work/body" "$work/headers"; rmdir "$work"' EXIT
request() {
  local expected=$1 method=$2 path=$3
  shift 3
  local actual
  actual=$(curl --max-time 15 -sS -D "$work/headers" -o "$work/body" -w '%{http_code}' -X "$method" "$base$path" "$@")
  printf '%s %s -> %s (expected %s)\n' "$method" "$path" "$actual" "$expected"
  cat "$work/body"
  printf '\n'
  test "$actual" = "$expected"
  if ((expected >= 400)); then
    grep -qi 'content-type: application/problem+json' "$work/headers"
  fi
}
request 200 GET /health
request 200 GET /ready
request 201 POST /api/v1/trips -H 'Content-Type: application/json' -d "$payload"
id=$(jq -er '.id' "$work/body")
jq -e '.status == "active"' "$work/body" >/dev/null
grep -qi "location: /api/v1/trips/$id" "$work/headers"
request 409 POST /api/v1/trips -H 'Content-Type: application/json' -d "$payload"
jq -e '.code == "driver_busy"' "$work/body" >/dev/null
request 200 GET "/api/v1/trips/$id"
request 200 POST "/api/v1/trips/$id/finish"
jq -e '.status == "completed" and .finished_at != null' "$work/body" >/dev/null
request 409 POST "/api/v1/trips/$id/finish"
jq -e '.code == "trip_completed"' "$work/body" >/dev/null
request 404 GET "/api/v1/trips/$missing"
jq -e '.code == "trip_not_found"' "$work/body" >/dev/null
request 404 POST "/api/v1/trips/$missing/finish"
request 400 POST /api/v1/trips -H 'Content-Type: application/json' -d '{'
jq -e '.code == "invalid_request"' "$work/body" >/dev/null
request 400 GET /api/v1/trips/not-a-uuid
request 400 POST /api/v1/trips -H 'Content-Type: application/json' -d "$(jq '.price = -1' <<<"$payload")"
request 400 POST /api/v1/trips -H 'Content-Type: application/json' -d "$(jq '.start_point.latitude = 91' <<<"$payload")"
printf 'PASS; completed demo trip: %s\n' "$id"