#!/usr/bin/env bash
# Pull OpenCost allocation into TimescaleDB as FOCUS-shaped rows.
# OpenCost API listens on :9003; :9090 is the UI (HTML) — do not sync from 9090.
set -euo pipefail

NS="${GIX_FINOPS_NS:-finops}"
WINDOW="${OPENCOST_WINDOW:-1d}"
OPENCOST_URL="${OPENCOST_URL:-http://127.0.0.1:9003}"
DATABASE_URL="${DATABASE_URL:-postgres://gixuser:change-me-local-only@127.0.0.1:5433/gix_rates?sslmode=disable}"
PF_PID=""

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing dependency: $1" >&2
    exit 1
  }
}

cleanup() {
  [[ -n "${PF_PID}" ]] && kill "${PF_PID}" 2>/dev/null || true
}
trap cleanup EXIT

need curl
need jq

ensure_api() {
  local url="$1"
  local body
  body="$(curl -sS --max-time 5 "${url}" 2>/dev/null || true)"
  if [[ "${body}" == \{* ]]; then
    echo "${body}"
    return 0
  fi
  return 1
}

ALLOC_PATH="/allocation/compute?window=${WINDOW}&aggregate=namespace,controller"
ALLOC_URL="${OPENCOST_URL%/}${ALLOC_PATH}"

echo "==> Fetching OpenCost allocation (${ALLOC_URL})"
RAW=""
if RAW="$(ensure_api "${ALLOC_URL}")"; then
  :
elif command -v kubectl >/dev/null 2>&1; then
  echo "    local API not up — starting: kubectl -n ${NS} port-forward svc/opencost 9003:9003"
  kubectl -n "${NS}" port-forward svc/opencost 9003:9003 >/dev/null 2>&1 &
  PF_PID=$!
  sleep 2
  ALLOC_URL="http://127.0.0.1:9003${ALLOC_PATH}"
  if ! RAW="$(ensure_api "${ALLOC_URL}")"; then
    echo "OpenCost API unreachable on :9003." >&2
    echo "UI is on :9090 (HTML). API port-forward:" >&2
    echo "  kubectl -n ${NS} port-forward svc/opencost 9003:9003" >&2
    exit 1
  fi
else
  echo "OpenCost API unreachable. Port-forward the API port (not UI):" >&2
  echo "  kubectl -n ${NS} port-forward svc/opencost 9003:9003" >&2
  exit 1
fi

if ! echo "${RAW}" | jq -e '.code == 200 or .data' >/dev/null 2>&1; then
  echo "OpenCost returned non-JSON / unexpected payload (got UI HTML?). Use port 9003." >&2
  echo "${RAW}" | head -c 200 >&2
  echo >&2
  exit 1
fi

TMP="$(mktemp)"
SQL="$(mktemp)"
trap 'rm -f "${TMP}" "${SQL}"; cleanup' EXIT

# OpenCost: .data[] is a map of name → allocation object
echo "${RAW}" | jq -r '
  [.data[]? | to_entries[]?]
  | .[]
  | .key as $k
  | .value as $v
  | [
      (($v.totalCost // $v.cpuCost // $v.ramCost // 0) | tonumber),
      ($v.properties.namespace // $v.name // $k // "unknown"),
      ($v.properties.controller // $v.properties.controllerKind // $v.name // $k // "workload")
    ]
  | @tsv
' | awk -F'\t' '$1+0 > 0 {print}' > "${TMP}"

ROWS="$(wc -l < "${TMP}" | tr -d ' ')"
echo "==> Preparing ${ROWS} OpenCost FOCUS inserts"

{
  cat <<'EOSQL'
CREATE TABLE IF NOT EXISTS focus_cost_records (
    time TIMESTAMPTZ NOT NULL,
    provider_name VARCHAR(50) NOT NULL DEFAULT 'OpenCost',
    source VARCHAR(50) NOT NULL,
    billed_cost NUMERIC(15, 10),
    effective_cost NUMERIC(15, 10) NOT NULL,
    service_category VARCHAR(50),
    resource_name VARCHAR(100),
    x_application VARCHAR(50) DEFAULT 'gix',
    x_team VARCHAR(50) DEFAULT 'platform',
    x_environment VARCHAR(50) DEFAULT 'local',
    charge_description TEXT
);
SELECT create_hypertable('focus_cost_records', 'time', if_not_exists => TRUE);
BEGIN;
EOSQL
  while IFS=$'\t' read -r cost ns ctrl; do
    [[ -z "${cost}" ]] && continue
    team="app"
    [[ "${ns}" == "finops" || "${ns}" == kube-* ]] && team="platform"
    esc_ctrl="${ctrl//\'/\'\'}"
    esc_ns="${ns//\'/\'\'}"
    printf "INSERT INTO focus_cost_records (time, provider_name, source, billed_cost, effective_cost, service_category, resource_name, x_application, x_team, x_environment, charge_description) VALUES (NOW(), 'OpenCost', 'opencost', %s, %s, 'Compute', '%s', 'gix', '%s', 'local', 'OpenCost allocation %s/%s');\n" \
      "${cost}" "${cost}" "${esc_ctrl}" "${team}" "${esc_ns}" "${esc_ctrl}"
  done < "${TMP}"

  cat <<'EOSQL'
INSERT INTO focus_cost_records
  (time, provider_name, source, billed_cost, effective_cost, service_category, resource_name, x_application, x_team, x_environment, charge_description)
SELECT
  p.time,
  'GixEstimator',
  'app-estimator',
  p.estimated_cost_usd,
  p.estimated_cost_usd,
  COALESCE(p.service_category, 'Compute'),
  COALESCE(p.resource_name, p.provider_id),
  'gix',
  'app',
  'local',
  'Unit cost for ' || p.provider_id
FROM provider_unit_costs p
WHERE p.time > NOW() - INTERVAL '1 day'
  AND NOT EXISTS (
    SELECT 1 FROM focus_cost_records f
    WHERE f.source = 'app-estimator'
      AND f.time = p.time
      AND f.resource_name = COALESCE(p.resource_name, p.provider_id)
  );

-- Mirror OpenCost FOCUS → unit table so Grafana unit-spend panels are not empty
-- when cost-estimator has not written yet (local FOSS loop).
INSERT INTO provider_unit_costs
  (time, provider_id, scraper_type, duration_ms, estimated_cost_usd, service_category, resource_name)
SELECT
  f.time,
  'opencost:' || COALESCE(f.resource_name, 'cluster'),
  'opencost',
  0,
  f.effective_cost,
  COALESCE(f.service_category, 'Compute'),
  COALESCE(f.resource_name, 'cluster')
FROM focus_cost_records f
WHERE f.source = 'opencost'
  AND f.time > NOW() - INTERVAL '7 days'
  AND NOT EXISTS (
    SELECT 1 FROM provider_unit_costs u
    WHERE u.time = f.time
      AND u.provider_id = 'opencost:' || COALESCE(f.resource_name, 'cluster')
  );
COMMIT;
EOSQL
} > "${SQL}"

run_psql() {
  if command -v psql >/dev/null 2>&1; then
    psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -f "${SQL}"
  elif docker ps --format '{{.Names}}' 2>/dev/null | grep -q '^gix-db$'; then
    docker exec -i gix-db psql -U "${POSTGRES_USER:-gixuser}" -d "${POSTGRES_DB:-gix_rates}" -v ON_ERROR_STOP=1 < "${SQL}"
  else
    echo "Need psql on PATH or a running gix-db container." >&2
    exit 1
  fi
}

run_psql
echo "=== Sync complete (${ROWS} OpenCost rows + app-estimator mirror) ==="
