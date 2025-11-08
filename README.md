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
    
    style A fill:#2d3748,stroke:#fff,stroke-width:3px,color:#fff
    style B fill:#4a5568,stroke:#cbd5e0,stroke-width:3px,color:#fff
    style C fill:#718096,stroke:#e2e8f0,stroke-width:2px,color:#fff
    style D fill:#718096,stroke:#e2e8f0,stroke-width:2px,color:#fff
    style E fill:#a0aec0,stroke:#e2e8f0,stroke-width:2px,color:#fff
```

### Data Flow

| Step | Action | Description |
|------|--------|-------------|
| **①** | **Request** | Frontend → API: `GET /api/v1/rates?cantor_id=1&currency=EUR` |
| **②** | **Cache Check** | API checks Redis for cached rates (60s TTL) |
| **③** | **Cache Result** | Hit: Return immediately / Miss: Query database |
| **④** | **Get Strategy** | Database returns scraping strategy (C1, C2, or C3) |
| **⑤** | **Scrape** | API executes strategy-specific scraper using Goquery |
| **⑥** | **HTML Response** | External cantor returns exchange rate data |
| **⑦** | **Cache Update** | Store fresh data in Redis (60s expiry) |
| **⑧** | **Archive** | Async save to TimescaleDB for historical analysis |
| **⑨** | **JSON Response** | API → Frontend: Return formatted exchange rates |

### Technology Stack

| Component | Technology | Purpose |
|-----------|-----------|---------|
| **Frontend** | Go + Gio UI | Native cross-platform desktop app |
| **Backend** | Go + net/http | REST API server with hot-reload (Air) |
| **Cache** | Redis | 60-second TTL for rate limiting scraping |
| **Database** | TimescaleDB | Time-series optimized PostgreSQL |
| **Scraping** | Goquery | Strategy Pattern for different cantor layouts |
| **Container** | Docker Compose | Single-command dev environment |

### Quick Start

```bash
# Terminal 1: Start backend (API + DB + Cache)
docker-compose up

# Terminal 2: Start frontend
go run ./cmd/gix/main.go
```

The API will be available at `http://localhost:8080` and the desktop app will connect automatically.
