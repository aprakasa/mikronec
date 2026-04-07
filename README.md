# Mikronec

[![Release](https://img.shields.io/github/v/release/aprakasa/mikronec?include_prereleases)](https://github.com/aprakasa/mikronec/releases)
[![Docker](https://img.shields.io/badge/ghcr.io-aprakasa%2Fmikronec-blue)](https://github.com/aprakasa/mikronec/pkgs/container/mikronec)
[![Go Reference](https://pkg.go.dev/badge/github.com/aprakasa/mikronec.svg)](https://pkg.go.dev/github.com/aprakasa/mikronec)
[![Go Report Card](https://goreportcard.com/badge/github.com/aprakasa/mikronec)](https://goreportcard.com/report/github.com/aprakasa/mikronec)
[![Tests](https://img.shields.io/badge/tests-passing-brightgreen)](Makefile)
[![Build](https://img.shields.io/badge/build-passing-brightgreen)](Makefile)
[![License](https://img.shields.io/github/license/aprakasa/mikronec)](LICENSE)

> 🇬🇧 English | [🇮🇩 Bahasa Indonesia](README_ID.md)

Mikronec (Mikrotik Connector) is a high-performance Go backend server that acts as a bridge to manage and monitor multiple MikroTik routers through a single secure API.

This application uses connection multiplexing to reuse existing connections and provides Server-Sent Events (SSE) endpoints for real-time data monitoring (such as hardware status and active users).

**Swagger UI**: `http://localhost:8080/swagger/index.html`

## Features

- **Multi-Router Management**: Manage many routers (router_id) from a single server.
- **Connection Multiplexing**: Connection sessions (based on host|user|pass) are shared across routers to save resources.
- **Real-time SSE**: `/sse/{routerID}` endpoint streams live data (CPU, Hotspot Active, PPP Active) to your frontend.
- **Robust Poller**: Data pollers auto-start, stop (when no clients), and reconnect if connection is lost.
- **Secure**: All endpoints are protected by static API Key middleware.

## Architecture

### System Overview

```mermaid
flowchart TB
    subgraph Client["Client Layer"]
        Web["Web Frontend"]
        Mobile["Mobile App"]
        CLI["CLI Tool"]
    end

    subgraph API["API Layer"]
        Router["HTTP Router\n(Go 1.26)"]
        Auth["API Key Auth"]
        Swagger["Swagger UI\n/docs"]
    end

    subgraph Core["Core Layer"]
        Manager["Router Manager"]
        SessionPool["Session Pool\n(host|user|pass)"]
        Poller["Auto Poller\n(2s interval)"]
        SSEHub["SSE Hub"]
    end

    subgraph Devices["MikroTik Devices"]
        MT1["Router 1\n192.168.1.1"]
        MT2["Router 2\n192.168.2.1"]
        MT3["Router N\n..."]
    end

    Web -->|HTTP/SSE| Router
    Mobile -->|HTTP/SSE| Router
    CLI -->|HTTP/SSE| Router
    
    Router --> Auth
    Auth --> Manager
    
    Manager --> SessionPool
    SessionPool -->|Connection| MT1
    SessionPool -->|Connection| MT2
    SessionPool -->|Connection| MT3
    
    Manager --> Poller
    Poller -->|Poll Data| MT1
    Poller -->|Poll Data| MT2
    Poller -->|Poll Data| MT3
    
    Poller --> SSEHub
    SSEHub -->|Real-time Events| Web
    SSEHub -->|Real-time Events| Mobile
```

### Connection Multiplexing

Multiple `router_id`s can share the same underlying connection when credentials match:

```mermaid
graph LR
    subgraph RouterIDs["Router IDs"]
        R1["router-01"]
        R2["router-02"]
        R3["router-03"]
        R4["router-04"]
    end

    subgraph Sessions["Session Pool"]
        SK1["Session Key A\n192.168.1.1\|admin\|pass"]
        SK2["Session Key B\n192.168.2.1\|admin\|pass"]
    end

    subgraph Connections["Active Connections"]
        C1["Connection 1"]
        C2["Connection 2"]
    end

    R1 -->|same key| SK1
    R2 -->|same key| SK1
    R3 --> SK2
    R4 -->|different| SK2
    
    SK1 --> C1
    SK2 --> C2
```

### Data Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant API as API Server
    participant M as Router Manager
    participant S as Session
    participant MT as MikroTik
    participant P as Poller
    participant SSE as SSE Hub

    C->>API: POST /connect
    API->>M: ConnectRouter()
    M->>S: Create/Get Session
    S->>MT: Establish Connection
    M->>P: Start Polling
    
    loop Every 2 seconds
        P->>MT: /system/resource/print
        P->>MT: /ip/hotspot/active/print
        P->>MT: /ppp/active/print
        MT-->>P: Data
        P->>SSE: Broadcast
    end
    
    C->>API: GET /sse/{routerID}
    API->>SSE: Subscribe
    SSE-->>C: Stream Events
    
    C->>API: POST /disconnect
    API->>M: StopRouter()
    M->>P: Stop Polling
    M->>SSE: Close Clients
```

## Configuration

This application is configured using environment variables.

- `MY_API_KEY` (Required): Secret key that must be sent by clients in the `X-API-Key` header for authentication. Server will fail to startup if this variable is not set.

- `PORT`: Port where the server will run. Default is 8080 (suitable for Google Cloud Run).

## How to Run

### 1. Local (Development)

Make sure you have Go (version 1.21+) installed.

```bash
# Set environment variables and run server
export MY_API_KEY="your-super-secret-api-key"
export PORT="8080"

go run .
# Output: ✅ MikroHot Connector (Secure) running at :8080
```

### 2. Docker (Production)

See `Dockerfile` below. You can build and run it with the following commands:

```bash
# 1. Build Docker image
docker build -t mikronec .

# 2. Run container
docker run -d -p 8080:8080 \
  -e MY_API_KEY="your-super-secret-api-key" \
  -e PORT="8080" \
  --name connector \
  mikronec
```

## API Usage

All API requests must include the `X-API-Key` and `ALLOWED_ORIGINS` headers.

```bash
X-API-Key: your-super-secret-api-key
ALLOWED_ORIGINS: http://localhost:8080
```

---

### POST /connect

Registers a `router_id` to specific router credentials and starts the connection.

Body (JSON):

```json
{
  "router_id": "router-01",
  "host": "192.168.88.1:8728",
  "user": "admin",
  "pass": "password123"
}
```

Example (cURL):

```bash
curl -X POST 'http://localhost:8080/connect' \
-H 'X-API-Key: your-super-secret-api-key' \
-H 'Content-Type: application/json' \
-d '{
  "router_id": "router-01",
  "host": "192.168.88.1",
  "user": "admin",
  "pass": "password123"
}'
```

---

### POST /disconnect

Closes the connection for a router_id, stops the poller, and closes all related SSE clients.

Body (JSON):

```json
{
  "router_id": "router-01"
}
```

Example (cURL):

```bash
curl -X POST 'http://localhost:8080/disconnect' \
-H 'X-API-Key: your-super-secret-api-key' \
-H 'Content-Type: application/json' \
-d '{"router_id": "router-01"}'
```

---

### GET /system-info

Fetches basic system information from the router associated with the `router_id`.

Query Parameter: `router` (string, required): The `router_id` you want to check.

Example (cURL):

```bash
curl 'http://localhost:8080/system-info?router=cabang-jakarta-01' \
-H 'X-API-Key: your-super-secret-api-key'
```

---

### POST /run

Executes an arbitrary command on the router. This is a very powerful and dangerous endpoint if your API Key is leaked.

Body (JSON):

- `router_id` (string): Target router.
- `args` (array of string): Command and arguments to execute.

Example (cURL):

```bash
# Example to fetch list of IP addresses
curl -X POST 'http://localhost:8080/run' \
-H 'X-API-Key: your-super-secret-api-key' \
-H 'Content-Type: application/json' \
-d '{
  "router_id": "cabang-jakarta-01",
  "args": ["/ip/address/print"]
}'
```

---

### GET /sse/{routerID}

Opens a Server-Sent Events (SSE) connection for real-time data streaming.

Path Parameter: `routerID` (string, required): The `router_id` you want to monitor.

Example (cURL):

```bash
# -N (no-buffer) option is required to see the stream
curl -N 'http://localhost:8080/sse/cabang-jakarta-01' \
-H 'X-API-Key: your-super-secret-api-key'
```

Example Stream Output:

```bash
data: {"router_id":"cabang-jakarta-01","hardware":[...],"hotspot_active":[],"ppp_active":[...],"ts":1678886400}

data: {"router_id":"cabang-jakarta-01","hardware":[...],"hotspot_active":[],"ppp_active":[...],"ts":1678886402}

...
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Development

### Setup

Install pre-commit hooks for code quality:

```bash
make setup      # Install pre-commit hooks
```

### Available Commands

```bash
make swag       # Generate swagger docs
make swag-fmt   # Format swagger annotations
make build      # Build binary
make test       # Run tests
make run        # Run server locally
make lint       # Run linters (gofmt, go vet, golint)
make fmt        # Format Go code
make clean      # Remove build artifacts
```

### Pre-commit Hooks

This project uses [pre-commit](https://pre-commit.com/) to ensure code quality:

- **Go**: `gofmt`, `go vet`, `goimports`, `golint`
- **Files**: Trailing whitespace, EOF fixer, YAML/JSON validation
- **Security**: Private key detection, large file check
- **Markdown**: Prettier formatting

Hooks run automatically on every commit. To run manually:

```bash
pre-commit run --all-files
```
