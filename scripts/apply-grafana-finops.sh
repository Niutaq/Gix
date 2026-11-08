#!/usr/bin/env bash
# Apply FinOps Grafana with Timescale credentials from .env (never commit real passwords).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NS="${GIX_NS:-${GIX_FINOPS_NS:-gix}}"
ENV_FILE="${ROOT}/.env"

if [[ -f "${ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  set -a
  # shellcheck disable=SC1091
  source "${ENV_FILE}"
  set +a
fi

PG_USER="${POSTGRES_USER:-gixuser}"
PG_DB="${POSTGRES_DB:-gix_rates}"
PG_PASS="${POSTGRES_PASSWORD:-change-me-local-only}"
# Local demo default; override with GRAFANA_ADMIN_PASSWORD (never commit real secrets).
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-gix-finops-local}"

TMP="$(mktemp)"
trap 'rm -f "${TMP}"' EXIT

# Render values with live DB credentials (uid "timescale" must match cost-flow.json).
python3 - "${ROOT}/k8s/finops/values-grafana.yaml" "${TMP}" "${PG_USER}" "${PG_DB}" "${PG_PASS}" <<'PY'
import sys
from pathlib import Path

src, dst, user, db, password = sys.argv[1:6]
text = Path(src).read_text(encoding="utf-8")
# Replace the placeholder block under TimescaleDB (keeps file reviewable without secrets).
old = """        database: gix_rates
        user: gixuser
        secureJsonData:
          password: change-me-local-only"""
new = f"""        database: {db}
        user: {user}
        secureJsonData:
          password: {password}"""
if old not in text:
    raise SystemExit("values-grafana.yaml Timescale placeholder block not found")
Path(dst).write_text(text.replace(old, new), encoding="utf-8")
PY

kubectl -n "${NS}" create configmap gix-finops-dashboards \
  --from-file=cost-flow.json="${ROOT}/k8s/finops/dashboards/cost-flow.json" \
  --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install grafana grafana/grafana \
  -n "${NS}" \
  -f "${TMP}" \
  --set "adminPassword=${GRAFANA_ADMIN_PASSWORD}" \
  --wait --timeout 5m

kubectl -n "${NS}" rollout restart deploy/grafana
kubectl -n "${NS}" rollout status deploy/grafana --timeout=120s

echo "Grafana FinOps ready → http://127.0.0.1:3000  (admin / ${GRAFANA_ADMIN_PASSWORD})"
echo "Dashboard: Dashboards → FinOps → Gix FinOps — Cost Flow"
echo "If :3000 is down: task obs:ui"
