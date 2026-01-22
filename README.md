<div align="center">
    
<img src="appicon2.png" alt="GIX Logo" width="192"/>

# GIX

*Real-time Currency Exchange Monitor (PLN-based)*

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker&logoColor=white)](https://www.docker.com)
[![License](https://img.shields.io/badge/License-MIT-b45f00?style=flat)](LICENSE)

[![SonarQube Cloud](https://sonarcloud.io/images/project_badges/sonarcloud-dark.svg)](https://sonarcloud.io/summary/new_code?id=Niutaq_Gix)

---
</div>

## Architecture

```mermaid
graph TD
    A[Gio Frontend] -->|① dRPC/Protobuf| B[Gin API Server]
    B -->|② Cache Check| C[(Redis Cache)]
    C -->|③ Miss| D[(PostgreSQL/TimescaleDB)]
    D -->|④ Strategy| B
    B -->|⑤ Scrape| E[External Cantors]
    E -->|⑥ Data| B
    B -->|⑦ Store| C
    B -->|⑧ Archive| D
    B -->|⑨ Response| A
    
    subgraph Infrastructure
    F[DigitalOcean K8s]
    G[DockerHub]
    end

    style A fill:#b45f00,stroke:#ff8c00,stroke-width:3px,color:#fff
    style B fill:#d97706,stroke:#fbbf24,stroke-width:3px,color:#fff
    style C fill:#92400e,stroke:#b45f00,stroke-width:2px,color:#fff
    style D fill:#92400e,stroke:#b45f00,stroke-width:2px,color:#fff
    style E fill:#78350f,stroke:#92400e,stroke-width:2px,color:#fff
    style F fill:#0080FF,stroke:#0059b3,stroke-width:2px,color:#fff
    style G fill:#2496ED,stroke:#0059b3,stroke-width:2px,color:#fff
```

### Data Flow Pipeline


| Step  | Action            | Description                                             |
|:-----:|-------------------|---------------------------------------------------------|
| **①** | **Request**       | Frontend → API: dRPC call with Protobuf payload         |
| **②** | **Cache Check**   | API checks Redis for cached rates (60s TTL)             |
| **③** | **Cache Result**  | **Hit**: Return immediately / **Miss**: Query database  |
| **④** | **Get Strategy**  | Database returns scraping strategy (selectors & logic)  |
| **⑤** | **Scrape**        | API executes strategy-specific scraper using Goquery    |
| **⑥** | **HTML Response** | External cantor returns exchange rate data              |
| **⑦** | **Cache Update**  | Store fresh data in Redis (60s expiry)                  |
| **⑧** | **Archive**       | Async save to TimescaleDB (PGX) for historical analysis |
| **⑨** | **Response**      | API → Frontend: Protobuf encoded response via dRPC      |

## Technology Stack

|     Component      |     Technology     | Purpose                                         |
|:------------------:|:------------------:|-------------------------------------------------|
|    **Frontend**    |    Go + Gio UI     | Native cross-platform desktop application       |
| **API Framework**  |      Gin (Go)      | High-performance HTTP/REST API server           |
| **Communication**  |  dRPC + ProtoBuf   | Lightweight Protobuf-based RPC                  |
|     **Cache**      |       Redis        | 60-second TTL for rate limiting and performance |
|    **Database**    |    TimescaleDB     | Time-series optimized PostgreSQL                |
|   **DB Driver**    |        PGX         | PostgreSQL Driver and Toolkit for Go            |
|    **Scraping**    |      Goquery       | Strategy Pattern for parsing cantor layouts     |
| **Infrastructure** | DigitalOcean + K8s | Scalable Kubernetes-managed hosting             |
|   **Container**    | Docker + DockerHub | Containerized deployment and registry           |

## Quick Start

### Prerequisites

Make sure you have installed:
- [<img src="https://img.shields.io/badge/Docker_Desktop-2496ED?style=flat&logo=docker&logoColor=white" alt="Docker Desktop"/>](https://www.docker.com/products/docker-desktop/)
- [<img src="https://img.shields.io/badge/Go_1.25+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"/>](https://go.dev/doc/install)



### Launch

**1.** Clone the project:
```bash
git clone https://github.com/Niutaq/Gix.git
cd Gix
```
**2.** Install Task (once):
```bash
# using Go
go install github.com/go-task/task/v3/cmd/task@latest
```
**3.** Run using commands

| Action        | Command              | Description                               |
|:--------------|:---------------------|:------------------------------------------|
| **Run App**   | `task run`           | Runs frontend connected to live API       |
| **Build Mac** | `task build:macos`   | Creates `Gix.app` (fixes fonts & signing) |
| **Build Win** | `task build:windows` | Creates `gix.exe` with icon               |
| **Clean**     | `task clean`         | Removes build artifacts                   |

---

## Demo

<video src="https://github.com/user-attachments/assets/5951e506-e98c-434e-ba29-f09a70355072" width="100%" controls autoplay loop muted></video>

