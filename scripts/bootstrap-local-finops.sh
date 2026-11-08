#!/usr/bin/env bash
# Bootstrap a zero-cloud FinOps stack: k3d + Prometheus + OpenCost (CNCF) + Grafana.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# Cluster/namespace are product-branded (Gix), not "finops-only" — FinOps is one capability inside.
CLUSTER_NAME="${GIX_CLUSTER_NAME:-gix-local}"
CTX="k3d-${CLUSTER_NAME}"
NS="${GIX_NS:-gix}"

need() {
  local bin="$1"
  command -v "${bin}" >/dev/null 2>&1 || {
    echo "Missing dependency: ${bin}" >&2
    exit 1
  }
}

need docker
need k3d
need helm
need kubectl

echo "==> Ensuring k3d cluster '${CLUSTER_NAME}'"
if ! k3d cluster list | grep -q "^${CLUSTER_NAME} "; then
  k3d cluster create "${CLUSTER_NAME}" \
    --agents 0 \
    --servers 1 \
    --port "18080:80@loadbalancer" \
    --port "18443:443@loadbalancer" \
    --k3s-arg "--disable=traefik@server:0" \
    --wait
else
  echo "    cluster already exists"
fi

kubectl config use-context "${CTX}" >/dev/null
kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

echo "==> Helm repos"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm repo add opencost https://opencost.github.io/opencost-helm-chart >/dev/null
helm repo add grafana https://grafana.github.io/helm-charts >/dev/null
helm repo update >/dev/null

echo "==> Prometheus"
helm upgrade --install prometheus prometheus-community/prometheus \
  -n "${NS}" \
  -f "${ROOT}/k8s/finops/values-prometheus.yaml" \
  --wait --timeout 10m

echo "==> OpenCost"
helm upgrade --install opencost opencost/opencost \
  -n "${NS}" \
  -f "${ROOT}/k8s/finops/values-opencost.yaml" \
  --wait --timeout 10m

echo "==> Grafana (Timescale creds from .env)"
bash "${ROOT}/scripts/apply-grafana-finops.sh"

echo ""
echo "=== Local Gix observability stack ready (FOSS / CNCF) ==="
echo "kubectl context : ${CTX}  (namespace: ${NS})"
echo "OpenCost UI     : kubectl -n ${NS} port-forward svc/opencost 9090:9090"
echo "                 then open http://localhost:9090"
echo "OpenCost API    : kubectl -n ${NS} port-forward svc/opencost 9003:9003"
echo "                 http://localhost:9003/allocation/compute?window=1d&aggregate=namespace"
echo "                 (API is :9003 — UI HTML is :9090; finops:sync uses :9003)"
echo "Grafana         : kubectl -n ${NS} port-forward svc/grafana 3000:80"
echo "                 login admin / gix-finops-local  (NodePort 30300 also)"
echo ""
echo "Next: task finops:sync   # pull OpenCost → Timescale FOCUS records"
echo "      task finops:flow   # curl cost-flow JSON from local API"
