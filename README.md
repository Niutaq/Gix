<div align="center">

<!-- Logo placeholder - replace with your actual logo -->
<img src="appicon.png" alt="GIX Logo" width="200"/>

# GIX

*Real-time Currency Exchange Monitor*

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker&logoColor=white)](https://www.docker.com)
[![License](https://img.shields.io/badge/License-MIT-b45f00?style=flat)](LICENSE)

</div>

---

## Architecture

```mermaid
graph LR
    A[Frontend] -->|① Request| B[API Server]
    B -->|② Check| C[(Cache)]
    C -->|③ Miss| D[(Database)]
    D -->|④ Strategy| B
    B -->|⑤ Scrape| E[Cantors]
    E -->|⑥ Data| B
    B -->|⑦ Store| C
    B -->|⑧ Archive| D
    B -->|⑨ Response| A
    
    style A fill:#b45f00,stroke:#ff8c00,stroke-width:3px,color:#fff
    style B fill:#d97706,stroke:#fbbf24,stroke-width:3px,color:#fff
    style C fill:#92400e,stroke:#b45f00,stroke-width:2px,color:#fff
    style D fill:#92400e,stroke:#b45f00,stroke-width:2px,color:#fff
    style E fill:#78350f,stroke:#92400e,stroke-width:2px,color:#fff
```

### Data Flow Pipeline

<div align="center">

| Step | Action | Description |
|:----:|--------|-------------|
| **①** | **Request** | Frontend → API: `ex. GET /api/v1/rates?cantor_id=1&currency=EUR` |
| **②** | **Cache Check** | API checks Redis for cached rates (60s TTL) |
| **③** | **Cache Result** | **Hit**: Return immediately / **Miss**: Query database |
| **④** | **Get Strategy** | Database returns scraping strategy (C1, C2, or C3) |
| **⑤** | **Scrape** | API executes strategy-specific scraper using Goquery |
| **⑥** | **HTML Response** | External cantor returns exchange rate data |
| **⑦** | **Cache Update** | Store fresh data in Redis (60s expiry) |
| **⑧** | **Archive** | Async save to TimescaleDB for historical analysis |
| **⑨** | **JSON Response** | API → Frontend: Return formatted exchange rates |

</div>

## Technology Stack

<div align="center">

| Component | Technology | Purpose |
|:---------:|:----------:|---------|
| **Frontend** | Go + Gio UI | Native cross-platform desktop app |
| **Backend** | Go + net/http | REST API server with hot-reload (Air) |
| **Cache** | Redis | 60-second TTL for rate limiting scraping |
| **Database** | TimescaleDB | Time-series optimized PostgreSQL |
| **Scraping** | Goquery | Strategy Pattern for different cantor layouts |
| **Container** | Docker Compose | Single-command dev environment |

</div>

## Quick Start

### Prerequisites

Make sure you have installed:
- [<img src="https://img.shields.io/badge/Docker_Desktop-2496ED?style=flat&logo=docker&logoColor=white" alt="Docker Desktop"/>](https://www.docker.com/products/docker-desktop/)
- [<img src="https://img.shields.io/badge/Go_1.25+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"/>](https://go.dev/doc/install)

### Launch

**1.** Start Docker Desktop

**2.** Run the application:

```bash
# Terminal 1: Start backend (API + DB + Cache)
docker-compose up

# Terminal 2: Start frontend
go run ./cmd/gix/main.go
```

---

## Demo

<div align="center">

![Demo Video](demos/gix_demo_1.gif)

</div>

---
</div>
