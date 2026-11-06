# FinOps Strategy for Gix

Gix practices **real FinOps** on a **CNCF / FOSS** stack that runs at `$0` for day-to-day work. Cloud invoices are optional reference material, not a requirement.

## Hosting context

| Era | Runtime | Notes |
| :--- | :--- | :--- |
| **Past** | **DigitalOcean** (DOKS + managed DB) | Backend was **deployed and exposed** here while DO credits lasted (~$27/mo vs ~$128/mo AWS EKS estimate at this scale). |
| **Past** | **Datadog** (APM / tracing) | Used to **analyze** scrapers, API latency, and unit spend; `dd-trace-go` integration remains optional via `DD_*`. |
| **Now** | Docker Compose + optional **k3d** | No cloud credits needed. FinOps / observability front door: OpenCost, Prometheus, Grafana, TimescaleDB (+ Loki). |
| **Reference** | `terraform/` (AWS) + Infracost | Kept for IaC cost modelling / learning — not the default deploy target. |

We are not “erasing” DO or Datadog — they validated the product under real hosting and APM. The active choice is to **replace the bill** with CNCF/FOSS equivalents for day-to-day work.

## Stack (local, FOSS)

```
scrapers / API ──► provider_unit_costs (Timescale)
                         │
k3d cluster ──► Prometheus ──► OpenCost ──► focus_cost_records (FOCUS-shaped)
                         │
                      Grafana (cost-flow dashboard)
```

Bootstrap:

```bash
task finops:bootstrap   # scripts/bootstrap-local-finops.sh
task finops:sync        # OpenCost allocation → Timescale
task obs:ui             # Grafana → http://127.0.0.1:3000  (admin / gix-finops-local)
task obs:grafana-sync   # re-apply dashboards + Timescale password from .env
task finops:flow        # GET /api/v1/finops/flow
```

Grafana reads Timescale via `host.k3d.internal:5433` — Postgres must publish `5433` on the host (not only `127.0.0.1`), and credentials must match `.env`. Empty panels usually mean: wrong DB password, closed port, or no rows in the last 24h (`task finops:sync` + running `cost-estimator` / scrapes).

Helm values live under `k8s/finops/`:
- `values-prometheus.yaml` — lightweight scrape stack
- `values-opencost.yaml` — CNCF OpenCost with **custom local rates**
- `values-grafana.yaml` — datasources + FinOps folder
- `dashboards/cost-flow.json` — spend / FOCUS / allocation panels

## FOCUS alignment

Application and cluster costs land in Timescale as FOCUS-shaped rows:

| Field | Source |
| :--- | :--- |
| `ChargePeriod` / `time` | scrape timestamp or OpenCost window |
| `ProviderName` | `OpenCost` or `GixEstimator` |
| `ServiceCategory` | AI / Compute / Network / … |
| `EffectiveCost` | estimated USD (local rates or unit model) |
| `x_Application` | `gix` |
| `x_Team` / `x_Environment` | tags / defaults (`platform`, `local`) |

Tables:
- `provider_unit_costs` — per-scraper unit economics (governance input)
- `focus_cost_records` — allocated / showback rows (OpenCost sync + app mirror)

## Governance

The in-process circuit breaker (`pkg/finops`) blocks providers whose **24h estimated spend** exceeds the configured limit (default `$0.05/day` per provider). That is application FinOps — complementary to OpenCost cluster allocation.

## Rates model

`DefaultRates` in `pkg/finops` mirrors the synthetic **local FOSS** rates used by OpenCost custom pricing (`k8s/finops/values-opencost.yaml`). Absolute dollars are for relative ranking and guardrails, not a provider invoice.

## What we deliberately avoid (for now)

- Paid APM as the primary cost UI (DataDog manifests may still exist; Grafana is the FinOps front door).
- Default deploys to DigitalOcean or AWS while credits are unavailable.
- Treating Infracost AWS estimates as “production truth” for this project’s day-to-day FinOps.

## Optional next steps

1. Export FOCUS CSV for offline analysis.
2. Wire Gio UI to `/api/v1/finops/flow` for interactive showback.
3. If a free remote cluster appears later, keep the **same** OpenCost/Prometheus charts — only change the kube-context.
