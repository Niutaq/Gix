# Gix - A Go-Powered Currency Rate Scraper

Gix is a desktop (_for now_) application built with **Gio UI** for monitoring currency exchange rates, powered by a containerized Go backend.

This project evolved from a simple monolith into a modern Client-Server architecture, capable of handling dozens of different, complex scraping strategies.

## Architecture

The system is now split into two core components:

1.  **Frontend (`cmd/gix/main.go`)**: A "thin" client application built with **Gio UI**. It contains no business logic. Its sole purpose is to render the UI and communicate with the backend via a REST API.
2.  **Backend (`cmd/gix-server/main.go`)**: A **Go REST API** server that manages all business logic, scraping, caching, and database interactions. It is fully containerized by Docker. - _**gRPC** or **other technique** is are planned to be implemented._

Data flow:
```mermaid
flowchart TD
    subgraph "Frontend (Your PC)"
        A[Gio UI App<br>(cmd/gix/main.go)]
    end

    subgraph "Backend (Running in Docker)"
        B(Gix Server / REST API<br>(cmd/gix-server/main.go))
        C[Redis Cache<br>(60s TTL)]
        D[TimescaleDB<br>(PostgreSQL)]
    end

    subgraph "External Cantors"
        E[Cantor 1 (Tadek)]
        F[Cantor 2 (Kwadrat)]
        G[Cantor 3 (Supersam)]
    end

    A -- HTTP GET --> B[/api/v1/rates?cantor_id=1]
    B -- 1. Check Cache --> C
    C -- 2. Cache Hit --> B
    B -- 3. Cache Miss --> D{Get Strategy<br>from 'cantors' table}
    D -- 4. 'C1' Strategy --> B
    B -- 5. Scrape (Goquery) --> E
    E -- 6. HTML --> B
    B -- 7. Store in Cache --> C
    B -- 8. Store in DB (async) --> D[rates table]
    B -- 9. Return JSON --> A
```

---

## Technology Stack

| Component | Technology | Purpose |
| :--- | :--- | :--- |
| **Frontend** | **Go (Gio)** | Renders the native, cross-platform UI. |
| **Backend** | **Go (net/http)** | Serves the REST API (`/api/v1/cantors`, `/api/v1/rates`). |
| **Environment** | **Docker & Docker Compose** | Runs the entire backend stack (API, DB, Cache) with one command. |
| **Database** | **TimescaleDB (PostgreSQL)** | Stores cantor configuration (strategies, URLs) and archives all historical rates. |
| **Cache** | **Redis** | Caches API responses for 60 seconds to improve performance and reduce scraping. |
| **Scraping** | **Goquery** | Used via the "Strategy Pattern" in `pkg/scrapers` to parse HTML. |
| **Dev Workflow** | **Air** | Provides live hot-reloading for the backend server inside its Docker container. |

---

## How to Run (Development)

Running the full application now requires two terminals.

### 1. Run the Backend

The backend (API, Database, Cache) is managed by Docker Compose.

```bash
# This command starts the Go API server (with Air hot-reload),
# the TimescaleDB database, and the Redis cache.
docker-compose up
```
The API server will be available at `http://localhost:8080`.

### 2. Run the Frontend (GUI)

In a **second terminal**, run the Gio application.

```bash
# This starts the desktop app, which will
# connect to the API server at localhost:8080.
go run ./cmd/gix/main.go
```

---

## Demos

<div align="center">
  <img src="demos/gix_demo_1.gif" alt="Gix Demo" width="600" />
</div>
