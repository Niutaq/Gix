<div align="center">
  <img src="appicon.png" alt="Gix Logo" width="150" />
  <h1>Gix</h1>
</div>

Gix is a distributed currency exchange rate monitor for the Polish cantor market: a native Gio UI (**`bin/Gix`**), a Go API (**`bin/Gix-API`**), heuristic scraping with an **offline LLM fallback**, and optional local **CNCF observability / FinOps** — no paid cloud required for day-to-day work.

## Table of Contents
- [Problem Statement](#problem-statement)
- [Architecture & Tech Stack](#architecture--tech-stack)
- [Hosting history & current direction](#hosting-history--current-direction)
- [Quick Start (fresh machine)](#quick-start-fresh-machine)
- [FinOps (CNCF / FOSS)](#finops-cncf--foss)
- [Offline LLM](#offline-llm)
- [Useful commands](#useful-commands)
- [Known Limitations](#known-limitations)
- [Roadmap](#roadmap)
- [Security & Contributing](#security--contributing)
- [Demo](#demo)

<img width="3904" height="1088" alt="gix_problem_and_solution" src="https://github.com/user-attachments/assets/c68b5057-dcaa-40ee-b628-e4d36c91a1a7" />

## Problem Statement
Physical cantors in Poland publish rates on fragmented, brittle websites. Gix aggregates them, streams updates live, and tracks the **unit cost** of scraping (compute / LLM) with FinOps guardrails.

## Architecture & Tech Stack

| Component | Technology | Description |
| :--- | :--- | :--- |
| **Frontend** | [![Gio](https://img.shields.io/badge/Gio-v0.9+-blue)](https://gioui.org) | Native, GPU-accelerated UI |
| **Backend API** | [![Go](https://img.shields.io/badge/Go-1.27.3-00ADD8?logo=go&logoColor=white)](https://go.dev/) [![Gin](https://img.shields.io/badge/Gin-v1.11-0088CC)](https://gin-gonic.com/) | HTTP API + harvester |
| **Streaming** | [![NATS](https://img.shields.io/badge/NATS_JetStream-27A16C?logo=nats.io&logoColor=white)](https://nats.io) | Event streaming / replay |
| **Caching** | [![Redis](https://img.shields.io/badge/Redis-v7-DC382D?logo=redis&logoColor=white)](https://redis.io) | Cache + rate update pub/sub |
| **Storage** | [![TimescaleDB](https://img.shields.io/badge/TimescaleDB-pg16-FDB515?logo=timescale&logoColor=black)](https://www.timescale.com) | Rates + FOCUS cost tables |
| **RPC** | [![dRPC](https://img.shields.io/badge/dRPC-Protobuf-4285F4)](https://storj.github.io/drpc/) | Live rates to the GUI |
| **Offline LLM** | [![Ollama](https://img.shields.io/badge/Ollama-local-000000?logo=ollama&logoColor=white)](https://ollama.com) | Scraper fallback — **no cloud API key** |
| **Local K8s** | [![k3d](https://img.shields.io/badge/k3d-k3s-FFBF00?logo=kubernetes&logoColor=black)](https://k3d.io) | Free cluster for FinOps demos |
| **FinOps** | [![OpenCost](https://img.shields.io/badge/OpenCost-CNCF-326CE5?logo=cncf&logoColor=white)](https://opencost.io) [![Prometheus](https://img.shields.io/badge/Prometheus-E6522C?logo=prometheus&logoColor=white)](https://prometheus.io) [![Grafana](https://img.shields.io/badge/Grafana-F46800?logo=grafana&logoColor=white)](https://grafana.com) | Allocation & cost-flow (FOSS) |
| **Legacy (kept)** | [![DigitalOcean](https://img.shields.io/badge/DigitalOcean-0080FF?logo=digitalocean&logoColor=white)](https://digitalocean.com) [![Datadog](https://img.shields.io/badge/Datadog-632CA6?logo=datadog&logoColor=white)](https://datadoghq.com) | Historical hosting + APM — see [Hosting history](#hosting-history--current-direction) |

## Hosting history & current direction

Gix was **analyzed and operated** with commercial tooling before the FOSS pivot — that history stays in the docs on purpose.

| Tool | What it was used for | What we use instead (FOSS / CNCF) |
| :--- | :--- | :--- |
| **DigitalOcean** (DOKS + managed services) | Hosted and exposed the production **backend** (API, harvester, DB wiring) while DO credits lasted | **Docker Compose** locally; optional **k3d**/k3s for a free cluster |
| **Datadog** (APM / tracing / dashboards) | Observability and ops **analysis** of scrapers and the API (`dd-trace-go` hooks remain in the tree) | **Prometheus + Grafana + OpenCost** (+ optional Loki) for metrics, logs, and FinOps showback |

**Current default path** (no paid cloud bill):

1. Docker Compose — TimescaleDB, Redis, NATS, Ollama
2. Optional k3d + OpenCost + Grafana — cluster FinOps
3. Terraform / Infracost under `terraform/` — IaC cost modelling reference only

Legacy helpers stay available but are archival: `task deploy:do` (DigitalOcean), Datadog agent manifests / `DD_*` env when you explicitly want that stack. Day-to-day development and demos target **CNCF / FOSS**.

More detail: **[FINOPS.md](FINOPS.md)**.

## Quick Start (fresh machine)

### 1. Prerequisites
- **Go** — version in [`go.mod`](go.mod)
- **Docker** + Compose (OrbStack / Docker Desktop / engine)
- **Task**: `go install github.com/go-task/task/v3/cmd/task@latest`
- For FinOps later: **k3d**, **helm**, **kubectl**, **jq**

### 2. Clone & env
```bash
git clone https://github.com/Niutaq/Gix.git
cd Gix
cp .env.example .env
```

`.env` already includes offline LLM defaults (uncommented):

```bash
LLM_PROVIDER=ollama
OLLAMA_URL=http://127.0.0.1:11434
OLLAMA_MODEL=llama3.2
```

No Gemini / MapTiler / Carto key is required to run. The map uses **OpenStreetMap** raster tiles (FOSS).

### 3. Infrastructure + offline LLM
```bash
docker compose up -d db redis nats ollama
task llm:pull          # one-time model download (large)
```

### 4. App (two terminals)
```bash
# terminal 1 — builds & runs bin/Gix-API
task run:backend

# terminal 2 — builds & runs bin/Gix (native Gio)
task start:gui:local
```

Or one shot (Compose + optional k3d FinOps + API + estimator): **`task up`**, then GUI in a second terminal.

- API: `http://127.0.0.1:8080` · dRPC: `localhost:8081`
- GUI: native Gio window (not a browser) — **Gio v0.9+ / v0.10.x**
- **Search**: type a Polish city → **Szukaj / Search** (or Enter). OSM discovery pins cantors nearby; rates fill in as scrapers resolve websites.
- **Locate**: when the search field is empty — GPS/IP for distance sorting.
- Language: macOS-style wheel picker in the sidebar / drawer.
- Empty quotes: soft grey pulse + localized “pending” hint (not a countdown).
## FinOps (CNCF / FOSS)

Details: **[FINOPS.md](FINOPS.md)**.

### Elevator pitch (what you are looking at)

Gix FinOps answers two different questions on purpose:

1. **How expensive is scraping cantors?** — every scrape emits a NATS event; `cost-estimator` writes a synthetic USD row to Timescale (`provider_unit_costs`). The Gio **Control Panel** shows the 24h burn (e.g. Compute vs Network). A circuit breaker blocks a cantor above **$0.05 / day**.
2. **How expensive is the local cluster?** — Prometheus + **OpenCost** allocate CPU/RAM on k3d using synthetic rates (not a cloud invoice). OpenCost Home is typically ~$0.04/day for `gix` + `kube-system`.

`task finops:sync` copies OpenCost allocation into FOCUS-shaped `focus_cost_records`. **Grafana** joins both worlds (cluster allocation + app unit costs). Absolute dollars are for ranking and guardrails; the cloud bill for day-to-day work stays **$0**.

<p align="center">
  <img src="demos/finops/opencost_home.png" alt="OpenCost UI — cluster cost allocation" width="720" />
  <br />
  <em>OpenCost (:9090) — cluster allocation by namespace (synthetic local rates)</em>
</p>

<p align="center">
  <img src="demos/finops/grafana_cost_flow.png" alt="Grafana Gix FinOps cost-flow dashboard" width="720" />
  <br />
  <em>Grafana (:3000) — FOCUS spend + app unit costs + cluster CPU</em>
</p>

Reading the OpenCost JSON at a glance: `totalCost` per namespace, `cpuCost` / `ramCost` from requests×rates, `cpuEfficiency` / `totalEfficiency` = usage vs request (low % ⇒ over-requested). Example: `gix` ≈ $0.022 + `kube-system` ≈ $0.022 → **~$0.04** cluster day — matches the UI. Local cluster/namespace defaults are **`gix-local` / `gix`** (product-branded; FinOps is a capability, not the whole stack).

### Run it

```bash
task up                 # preferred: Compose + k3d (gix-local / ns gix) + Gix-API + estimator + sync
task start:gui:local    # second terminal — bin/Gix
task finops:down        # stop background procs from task up
```

| Port | What |
| :--- | :--- |
| **9003** | OpenCost **API** (JSON) — used by `finops:sync` |
| **9090** | OpenCost **UI** (HTML) |
| **3000** | Grafana (`admin` / `gix-finops-local`) |
| **8080** | App API — `/api/v1/finops` (GUI burn) and `/api/v1/finops/flow` |

Do **not** point sync at `:9090` — that serves the UI, not the allocation API.

## Offline LLM

Scrapers use heuristics first; on failure they call **Ollama** locally (`LLM_PROVIDER=ollama`).

| Variable | Default | Meaning |
| :--- | :--- | :--- |
| `LLM_PROVIDER` | `ollama` | `ollama` \| `gemini` (legacy) \| `auto` |
| `OLLAMA_URL` | `http://127.0.0.1:11434` | Ollama HTTP API |
| `OLLAMA_MODEL` | `llama3.2` | Model name for `task llm:pull` |

```bash
task llm:up      # docker compose up -d ollama
task llm:pull    # ollama pull $OLLAMA_MODEL
```

Legacy cloud: set `LLM_PROVIDER=gemini` and `GEMINI_API_KEY=...` only if you explicitly want that path.

## Useful commands

| Command | Description |
| :--- | :--- |
| `task up` | Full local stack (API + optional FinOps) |
| `task run:backend` | `bin/Gix-API` + harvester |
| `task start:gui:local` | `bin/Gix` → local API |
| `task run:estimator` | Cost estimator worker |
| `task llm:up` / `task llm:pull` | Offline Ollama |
| `task finops:up` / `down` / `status` | Same as `task up` + lifecycle helpers |
| `task finops:bootstrap` | k3d OpenCost stack only |
| `task finops:sync` / `task finops:flow` | FOCUS sync + showback JSON |
| `task test` / `task lint` / `task vuln` | Quality |
| `task trivy:scan` | Vulnerability scan |
| `task deploy:do` | *(legacy)* DigitalOcean |

## Known Limitations
- First `task llm:pull` is large and needs disk + time.
- Heuristic scrapers still break when cantor HTML changes; Ollama is the fallback.
- City discovery depends on OpenStreetMap / Nominatim coverage — small towns may have few tagged kantors.
- OpenCost local USD uses synthetic rates (relative FinOps, not a cloud invoice).
- Elasticsearch is optional; if nothing listens on `:9200`, search sync is skipped.
- Native GPS may time out on macOS; IP geolocation is the fallback (can be coarse).

## Roadmap
- [x] Heuristic scrapers + offline Ollama LLM fallback
- [x] FinOps estimator, governance, local OpenCost/FOCUS sync
- [x] NATS JetStream + dRPC streaming
- [x] City search (Nominatim / Overpass) + language wheel UI
- [ ] FinOps cost-flow visualization inside the Gio GUI
- [ ] NetSecOps / NIS2 hardening (after FinOps UI)

## Security & Contributing
See [SECURITY.md](SECURITY.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## License
MIT License

## Demo

Native Gio client — city search, language picker, live EUR quotes, map pins:

[![Gix demo](demos/app_demo.gif)](https://github.com/Niutaq/Gix/releases/download/demo/app_demo.mp4)

Full video (mp4): [download / play](https://github.com/Niutaq/Gix/releases/download/demo/app_demo.mp4) · in-repo [`demos/app_demo.mp4`](demos/app_demo.mp4)
