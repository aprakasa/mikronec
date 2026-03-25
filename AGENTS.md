# AGENTS.md

Guidelines for agentic coding agents working in the Mikronec codebase.

## Project Overview

Mikronec is a Go backend service that manages multiple MikroTik routers through a unified API. It provides connection pooling, real-time SSE streaming, and command execution capabilities.

- **Language**: Go 1.26
- **Module**: `github.com/aprakasa/mikronek`
- **HTTP Framework**: Standard library `net/http` (no framework)
- **Architecture**: Clean separation with `cmd/` for entry points and `internal/` for private code

## Build Commands

```bash
make build          # Compile binary to bin/server
make run            # Run development server (requires API_KEY, ALLOWED_ORIGINS env vars)
make clean          # Remove bin/ directory
make swag           # Regenerate Swagger documentation
make swag-fmt       # Format Swagger annotations
```

## Test Commands

```bash
make test                           # Run all tests
go test ./...                       # Run all tests (alternative)
go test -v ./...                    # Run all tests with verbose output
go test -run TestName ./...         # Run specific test by name
go test -run TestAuth ./internal/middleware/...   # Run tests matching pattern in package
go test -v -run TestUptimeToSeconds ./internal/normalize/...  # Single test with verbose
```

## Code Style

### Imports

Group imports with blank lines between groups:
1. Standard library
2. Third-party packages
3. Local packages (`github.com/aprakasa/mikronek/internal/...`)

```go
import (
    "context"
    "net/http"
    
    "github.com/go-routeros/routeros/v3"
    
    "github.com/aprakasa/mikronek/internal/types"
)
```

### Comments

- Every file starts with a package comment: `// Package name provides/explains...`
- All exported functions/types have doc comments starting with the name
- Use complete sentences with proper punctuation

```go
// ConnectRouter establishes a connection to a RouterOS device.
// If a session already exists for the key, it validates the connection.
func (m *Manager) ConnectRouter(ctx context.Context, key, host, user, pass string) error {
```

### Naming Conventions

- **Exported**: PascalCase (`HandleConnect`, `SessionWrap`, `JSONResponse`)
- **Private**: camelCase (`sessions`, `sseHUB`, `pollerStop`)
- **Acronyms**: Uppercase (`ID`, `SSE`, `API`, `JSON`)
- **Constants**: camelCase for private, PascalCase for exported (`sseRetryMs`)
- **Interfaces**: Often end in `-er` (not used heavily in this codebase)
- **Request/Response types**: Suffix with `Request` or `Response` (`ConnectRequest`, `JSONResponse`)

### Types

- Use `any` instead of `interface{}`
- Use `int64` for numeric conversions from strings
- JSON struct tags use snake_case for API fields

```go
type ConnectRequest struct {
    RouterID string `json:"router_id"`
    Host     string `json:"host"`
}
```

- Prefer small, focused types over large structs
- Embed mutexes directly in structs that need thread-safety

### Error Handling

- Log internal errors with `log.Printf` for debugging
- Return sanitized/public messages to API clients
- Use `fmt.Errorf` for wrapped errors with context
- Ignore errors explicitly with `_` only when appropriate (e.g., `json.NewEncoder(w).Encode()`)

```go
if err != nil {
    log.Printf("Internal error (code %d): %s", code, internalMsg)
    return fmt.Errorf("router %s not connected", routerID)
}
```

### HTTP Handlers

- Use helper functions `JSONOK(w, data)` and `JSONErr(w, msg, code)` for responses
- Validate request body early and return on errors
- Extract path values with `r.PathValue("key")` (Go 1.22+)
- Use `r.Context().Done()` for client disconnect detection

```go
func HandleConnect(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
    var body types.ConnectRequest
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        JSONErr(w, "invalid json body", http.StatusBadRequest)
        return
    }
    // ... handler logic
    JSONOK(w, result)
}
```

### Concurrency

- Use `sync.Mutex` or `sync.RWMutex` with deferred unlock
- Prefer `m.mu.Lock(); defer m.mu.Unlock()` pattern at the start of critical sections
- Use `sync.Once` for one-time operations (e.g., closing channels)

```go
type SSEClient struct {
    Ch   chan SSEEvent
    once sync.Once
}

func (c *SSEClient) Close() {
    c.once.Do(func() { close(c.Ch) })
}
```

- For RWMutex, use `RLock()/RUnlock()` for reads, `Lock()/Unlock()` for writes

## Testing Guidelines

- Use table-driven tests with `t.Run` for subtests
- Use `httptest.NewRequest` and `httptest.NewRecorder` for HTTP handler tests
- Use `t.Setenv()` for environment variable tests (automatically cleaned up)
- Name test functions `TestFunctionName` or `TestFunctionName_Scenario`

```go
func TestAuthMiddleware(t *testing.T) {
    tests := []struct {
        name       string
        apiKey     string
        wantStatus int
    }{
        {"valid key", "test-api-key", http.StatusOK},
        {"invalid key", "wrong-key", http.StatusUnauthorized},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## Swagger/API Documentation

- Add Swagger annotations to all HTTP handlers
- Run `make swag` after adding/modifying annotations
- Required annotations: `@Summary`, `@Description`, `@Tags`, `@Param`, `@Success`, `@Failure`, `@Router`

```go
// @Summary Connect to a MikroTik router
// @Description Establish a connection to a MikroTik router.
// @Tags connection
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key"
// @Param request body types.ConnectRequest true "Connection details"
// @Success 200 {object} types.JSONResponse
// @Failure 400 {object} types.JSONResponse
// @Router /connect [post]
func HandleConnect(...) { }
```

## Project Structure

```
mikronec/
├── cmd/server/main.go       # Entry point, route definitions
├── internal/
│   ├── handler/             # HTTP handlers (handlers.go, sse.go)
│   ├── middleware/          # Auth, CORS middleware
│   ├── normalize/           # RouterOS response transformation
│   ├── router/              # Session manager, polling, SSE hub
│   └── types/               # Core data structures
├── docs/                    # Generated Swagger docs
├── Makefile                 # Build automation
└── go.mod                   # Dependencies
```

## Key Patterns

### Connection Pooling

Sessions are keyed by `host|user|pass` and shared across multiple router IDs:

```go
key := body.Host + "|" + body.Username + "|" + body.Password
rm.AddRouter(body.RouterID, key)
```

### Auto-start/stop Polling

Polling starts when first SSE client connects, stops when last disconnects:

```go
func (m *Manager) RemoveSSEClient(routerID string, client *types.SSEClient) {
    // ... remove client
    if len(m.sseHUB[routerID]) == 0 {
        close(stop) // auto-stop polling
    }
}
```

### Middleware Chain

```go
var h http.Handler = mux
h = middleware.AuthMiddleware(cfg.APIKey)(h)
h = middleware.CORSMiddleware(cfg.AllowedOrigins)(h)
```
