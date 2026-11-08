#!/usr/bin/env bash
# One-command local Gix stack: Compose → (optional) k3d observability → port-forwards → Gix-API → estimator → sync.
# Usage:
#   bash scripts/finops-stack.sh up      # sequential, verbose
#   bash scripts/finops-stack.sh down    # stop background procs started by up
#   bash scripts/finops-stack.sh status  # what's listening / PID files
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

STATE_DIR="${ROOT}/.finops"
PID_DIR="${STATE_DIR}/pids"
LOG_DIR="${ROOT}/logs"
NS="${GIX_NS:-${GIX_FINOPS_NS:-gix}}"
CLUSTER_NAME="${GIX_CLUSTER_NAME:-gix-local}"

mkdir -p "${PID_DIR}" "${LOG_DIR}"

step() { printf '\n==> %s\n' "$*"; }
ok()   { printf '    ✓ %s\n' "$*"; }
warn() { printf '    ! %s\n' "$*" >&2; }

need() {
  local bin="$1"
  command -v "${bin}" >/dev/null 2>&1 || {
    echo "Missing dependency: ${bin}" >&2
    exit 1
  }
}

load_env() {
  if [[ -f "${ROOT}/.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "${ROOT}/.env"
    set +a
  fi
  POSTGRES_USER="${POSTGRES_USER:-gixuser}"
  POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-change-me-local-only}"
  POSTGRES_DB="${POSTGRES_DB:-gix_rates}"
  export DATABASE_URL="${DATABASE_URL:-postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@127.0.0.1:5433/${POSTGRES_DB}?sslmode=disable}"
  export REDIS_URL="${REDIS_URL:-redis://localhost:6379/0}"
  export NATS_URL="${NATS_URL:-nats://localhost:4222}"
  export DD_TRACE_ENABLED="${DD_TRACE_ENABLED:-false}"
  export GOFLAGS="${GOFLAGS:--buildvcs=false}"
}

port_open() {
  local port="$1"
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "${port}" >/dev/null 2>&1
  else
    curl -sf --max-time 1 "http://127.0.0.1:${port}/" >/dev/null 2>&1 || return 1
  fi
}

wait_port() {
  local port="$1" label="$2" tries="${3:-30}"
  local i
  for ((i = 1; i <= tries; i++)); do
    if port_open "${port}"; then
      ok "${label} ready on :${port}"
      return 0
    fi
    sleep 1
  done
  warn "${label} not ready on :${port} after ${tries}s"
  return 1
}

write_pid() {
  local name="$1" pid="$2"
  echo "${pid}" > "${PID_DIR}/${name}.pid"
}

is_our_pid_alive() {
  local name="$1"
  local f="${PID_DIR}/${name}.pid"
  [[ -f "${f}" ]] || return 1
  local pid
  pid="$(cat "${f}")"
  kill -0 "${pid}" 2>/dev/null
}

stop_named() {
  local name="$1"
  local f="${PID_DIR}/${name}.pid"
  if [[ -f "${f}" ]]; then
    local pid
    pid="$(cat "${f}")"
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" 2>/dev/null || true
      sleep 0.3
      kill -9 "${pid}" 2>/dev/null || true
      ok "stopped ${name} (pid ${pid})"
    else
      warn "${name} pid ${pid} already dead"
    fi
    rm -f "${f}"
  fi
}

start_pf() {
  local name="$1" svc="$2" local_port="$3" remote_port="$4"
  if port_open "${local_port}"; then
    ok "port :${local_port} already in use — skipping ${name} port-forward"
    return 0
  fi
  if is_our_pid_alive "${name}"; then
    ok "${name} port-forward already tracked"
    return 0
  fi
  need kubectl
  kubectl -n "${NS}" port-forward "svc/${svc}" "${local_port}:${remote_port}" \
    >> "${LOG_DIR}/${name}.log" 2>&1 &
  write_pid "${name}" $!
  wait_port "${local_port}" "${name}" 20 || true
}

cmd_down() {
  step "Stopping FinOps background processes"
  stop_named grafana-pf
  stop_named opencost-pf
  stop_named opencost-ui-pf
  stop_named backend
  stop_named estimator
  ok "done (docker compose / k3d left running — stop those separately if needed)"
}

cmd_status() {
  step "FinOps status"
  for p in 5433 4222 6379 8080 8081 9003 9090 3000; do
    if port_open "${p}"; then
      ok ":${p} listening"
    else
      printf '    · :%s idle\n' "${p}"
    fi
  done
  echo ""
  echo "PID files in ${PID_DIR}:"
  if compgen -G "${PID_DIR}/*.pid" >/dev/null 2>&1; then
    for f in "${PID_DIR}"/*.pid; do
      local name pid
      name="$(basename "${f}" .pid)"
      pid="$(cat "${f}")"
      if kill -0 "${pid}" 2>/dev/null; then
        ok "${name} pid=${pid} alive"
      else
        warn "${name} pid=${pid} stale"
      fi
    done
  else
    echo "    (none)"
  fi
  echo ""
  echo "Logs: ${LOG_DIR}/backend.log ${LOG_DIR}/finops.log ${LOG_DIR}/grafana-pf.log ${LOG_DIR}/opencost-pf.log"
}

cmd_up() {
  load_env
  need docker
  need curl

  step "1/7 Docker Compose (db redis nats)"
  docker compose up -d db redis nats
  wait_port 5433 "Timescale" 40
  wait_port 4222 "NATS" 20 || true
  wait_port 6379 "Redis" 20 || true

  step "2/7 k3d FinOps cluster (Prometheus / OpenCost / Grafana)"
  if command -v k3d >/dev/null 2>&1 && command -v helm >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1; then
    if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
      ok "cluster '${CLUSTER_NAME}' already exists"
      kubectl config use-context "k3d-${CLUSTER_NAME}" >/dev/null 2>&1 || true
    else
      echo "    bootstrapping (first run can take several minutes)..."
      bash "${ROOT}/scripts/bootstrap-local-finops.sh"
    fi
  else
    warn "k3d/helm/kubectl missing — skipping cluster FinOps (app unit costs still work)"
  fi

  step "3/7 Port-forwards (Grafana :3000, OpenCost API :9003, OpenCost UI :9090)"
  if command -v kubectl >/dev/null 2>&1 && kubectl -n "${NS}" get svc grafana >/dev/null 2>&1; then
    start_pf grafana-pf grafana 3000 80
    start_pf opencost-pf opencost 9003 9003
    start_pf opencost-ui-pf opencost 9090 9090
  else
    warn "namespace '${NS}' / services not found — skip port-forwards (bootstrap first)"
  fi

  step "4/7 Gix API (bin/Gix-API :8080 / :8081)"
  if port_open 8080; then
    ok "Gix API already on :8080"
  else
    (
      cd "${ROOT}"
      mkdir -p bin
      go build -buildvcs=false -o bin/Gix-API ./cmd/gix-server
      env DATABASE_URL="${DATABASE_URL}" REDIS_URL="${REDIS_URL}" NATS_URL="${NATS_URL}" \
        DD_TRACE_ENABLED="${DD_TRACE_ENABLED}" GOFLAGS="${GOFLAGS}" \
        ./bin/Gix-API >> "${LOG_DIR}/backend.log" 2>&1
    ) &
    write_pid backend $!
    wait_port 8080 "Gix API" 90
  fi

  step "5/7 Cost estimator"
  if is_our_pid_alive estimator || pgrep -f 'cmd/cost-estimator|cost-estimator' >/dev/null 2>&1; then
    ok "estimator already running"
  else
    (
      cd "${ROOT}"
      env DATABASE_URL="${DATABASE_URL}" NATS_URL="${NATS_URL}" \
        DD_TRACE_ENABLED="${DD_TRACE_ENABLED}" GOFLAGS="${GOFLAGS}" \
        go run ./cmd/cost-estimator >> "${LOG_DIR}/finops.log" 2>&1
    ) &
    write_pid estimator $!
    sleep 2
    if is_our_pid_alive estimator; then
      ok "estimator started (pid $(cat "${PID_DIR}/estimator.pid"))"
    else
      warn "estimator may have exited — check logs/finops.log"
    fi
  fi

  step "6/7 OpenCost → FOCUS sync"
  if port_open 9003 || kubectl -n "${NS}" get svc opencost >/dev/null 2>&1; then
    if bash "${ROOT}/scripts/sync-opencost-focus.sh"; then
      ok "sync complete"
    else
      warn "sync failed (OpenCost may still be warming up) — retry: task finops:sync"
    fi
  else
    warn "skipping sync (no OpenCost)"
  fi

  step "7/7 Ready"
  echo ""
  echo "  API        http://127.0.0.1:8080"
  echo "  FinOps     http://127.0.0.1:8080/api/v1/finops"
  echo "  Flow       task finops:flow"
  echo "  Grafana    http://127.0.0.1:3000   (admin / gix-finops-local)"
  echo "  OpenCost UI http://127.0.0.1:9090"
  echo "  OpenCost API http://127.0.0.1:9003/allocation/compute?window=1d&aggregate=namespace"
  echo ""
  echo "  GUI (1 more terminal):  task start:gui:local"
  echo "  Stop stack:             task finops:down"
  echo "  Status:                 task finops:status"
  echo ""
  echo "  Logs: tail -f logs/backend.log logs/finops.log"
  cmd_status
}

case "${1:-}" in
  up) status=0; cmd_up || status=$?; exit "${status}" ;;
  down) cmd_down ;;
  status) load_env; cmd_status ;;
  *)
    echo "Usage: $0 {up|down|status}" >&2
    exit 2
    ;;
esac
