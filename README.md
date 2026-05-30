<div align="center">
  <img src="appicon.png" alt="Gix Logo" width="150" />
  <h1>Gix</h1>
</div>

Gix is a distributed currency exchange rate monitor designed specifically for the Polish cantor market. It leverages a fast vector-based UI (Gio) and a robust cloud-native backend to track, aggregate, and stream exchange rates in real-time.

---

## Table of Contents
- [Problem Statement](#problem-statement)
- [Architecture & Tech Stack](#architecture--tech-stack)
- [FinOps & Cost-Awareness (FOCUS Framework) - WORK IN PROGRESS](#finops--cost-awareness-focus-framework---work-in-progress)
- [Quick Start](#quick-start)
- [Known Limitations](#known-limitations)
- [Roadmap](#roadmap)
- [Security & Contributing](#security--contributing)
- [Demo](#demo)

---

## Problem Statement
The currency exchange market (specifically physical cantors in Poland) is highly fragmented. Rates change dynamically, spreads vary dramatically, and finding the best real-time deal requires scraping dozens of disjointed, poorly-optimized websites. 

Gix solves this by providing:
- **Centralized Intelligence**: Aggregates physical exchange office rates via smart, heuristic-based web scraping.
- **Zero-Lag Updates**: Uses NATS JetStream and dRPC for instant rate streaming directly to a native desktop/mobile client without polling.
- **Cost Transparency**: Scraping external providers isn't free (compute, bandwidth, LLM tokens). Gix includes a built-in.

<img width="1264" height="842" alt="gix_problem_solution" src="https://github.com/user-attachments/assets/5deb6270-7502-4927-bd07-cdfd5cb2d03a" />

## Architecture & Tech Stack

| Component | Technology / Version | Description |
| :--- | :--- | :--- |
| **Frontend** | [![Gio](https://img.shields.io/badge/Gio-v0.9.0-blue)](https://gioui.org) | Native, GPU-accelerated, cross-platform UI |
| **Backend API** | [![Go](https://img.shields.io/badge/Go-1.26.2-00ADD8?logo=go&logoColor=white)](https://go.dev/) [![Gin](https://img.shields.io/badge/Gin-v1.11-0088CC)](https://gin-gonic.com/) | High-performance routing and orchestration |
| **Streaming** | [![NATS](https://img.shields.io/badge/NATS_JetStream-v1.48-27A16C)](https://nats.io) | Persistent event streaming and replay |
| **Caching** | [![Redis](https://img.shields.io/badge/Redis-v7-DC382D?logo=redis&logoColor=white)](https://redis.io) | Fast access and rate limiting |
| **Storage** | [![TimescaleDB](https://img.shields.io/badge/TimescaleDB-pg16-FDB515)](https://www.timescale.com) | Time-series data for historical charting |
| **RPC** | [![dRPC](https://img.shields.io/badge/dRPC-Protobuf-4285F4)](https://storj.github.io/drpc/) | Lightweight streaming protocol |
| **Infra** | [![DOKS](https://img.shields.io/badge/DigitalOcean-K8s-0080FF?logo=digitalocean&logoColor=white)](https://digitalocean.com) | Cloud hosting |
| **Observability**| [![DataDog](https://img.shields.io/badge/DataDog-APM-632CA6?logo=datadog&logoColor=white)](https://datadoghq.com) | Traces, Metrics, Logs |

### Why this stack?
- **Go**: Provides great performance, simple concurrency (goroutines), and static typing, perfect for both scraping engines and lightweight microservices.
- **Gio UI**: Immediate-mode vector graphics allow 60 FPS rendering on native platforms without embedding an entire Chromium browser (like Electron).
- **TimescaleDB**: Native PostgreSQL extension highly optimized for time-series data, perfect for analyzing historical currency trends.
- **NATS JetStream**: Lightweight, high-performance event streaming that supports replayability and exactly-once delivery.

## FinOps & Cost-Awareness (FOCUS Framework) - WORK IN PROGRESS
It's designed with strict **FinOps principles**:
- **Visibility**: Real-time cost estimation per scraper run, saved directly to TimescaleDB (`provider_unit_costs` table).
- **Governance**: A built-in Circuit Breaker cuts off cantors if the cost-to-serve ratio exceeds `$0.05` per day.
- **Optimization**: Kubernetes resources are strictly bounded, TimescaleDB chunks are aggressively dropped after 30 days, and Redis handles traffic spikes to shield the DB.

## Quick Start

### Prerequisites
- **Go** 1.26.2+
- **Docker** & **Docker Compose**
- **Task** (`go install github.com/go-task/task/v3/cmd/task@latest`)
- **Gemini API Key**: Required for the Heuristic LLM fallback scraper. Set it as an environment variable:
  ```bash
  export GEMINI_API_KEY="your_api_key_here"
  ```

### Local Development
To start the entire environment (TimescaleDB, Redis, NATS) and run the Backend + UI natively:
```bash
task dev
```

### Remote (Cloud API)
To run the native UI connected to the production cloud API:
```bash
task start:gui:remote
```

### Available Commands
Below is a list of all commands configured in the `Taskfile.yml`:

| Command | Description |
| :--- | :--- |
| `task proto` | Generates Go code from Protobuf files |
| `task lint` | Runs `golangci-lint` |
| `task test` | Runs unit tests |
| `task vuln` | Runs `govulncheck` on Go code |
| `task run:backend` | Runs the backend locally |
| `task run:estimator` | Runs the cost estimator locally |
| `task start:gui:local` | Starts GUI pointing to the local API |
| `task start:gui:remote` | Starts GUI pointing to the remote DigitalOcean API |
| `task deploy:do` | Builds, pushes, and deploys backend to DigitalOcean K8s |
| `task docker:build` | Builds the docker image |
| `task k8s:deploy` | Deploys backend to Kubernetes |
| `task clean` | Cleans temporary and build files |
| `task build:macos` | Builds a valid `.app` package for macOS |
| `task helm:lint` | Lints Helm chart |
| `task trivy:scan` | Scans the project for vulnerabilities using Trivy |

## Known Limitations
- **Scraper Brittleness**: Scraping physical cantors relies on their HTML structure. If a cantor updates their site, the static scraper might break. The *Heuristic LLM fallback* mitigates this but consumes API tokens.
- **Geolocation API**: The fallback to OSM Nominatim for city search is rate-limited by OpenStreetMap's fair usage policy.

## Roadmap
- [x] Heuristic LLM-based Cantor Discovery (WIP)
- [x] NATS JetStream Event Streaming
- [ ] FinOps Cost-Estimator & Governance Circuit Breaker
- [ ] Predictive ML Anomaly Detection (DataDog ML)
- [ ] ...more???

## Security & Contributing
Please read our [SECURITY.md](SECURITY.md) for reporting vulnerabilities. 
If you want to contribute, check [CONTRIBUTING.md](CONTRIBUTING.md).

## License
MIT License

---

## Demo

https://github.com/user-attachments/assets/cd56a6cf-e0fe-4516-b890-a67a214c8e42

