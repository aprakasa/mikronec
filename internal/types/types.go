// Package types defines core data structures used throughout the Mikronec application.
// It includes session management types, SSE event structures, and API request/response types.
package types

import (
	"sync"

	"github.com/go-routeros/routeros/v3"
)

// SessionWrap wraps a RouterOS client connection with thread-safe access
// and stores connection credentials for potential reconnection.
type SessionWrap struct {
	Conn     *routeros.Client
	Mu       sync.Mutex
	Host     string
	Username string
	Password string
}

// SSEEvent represents a Server-Sent Event with ID, event type, and data payload.
type SSEEvent struct {
	ID    string
	Event string
	Data  []byte
}

// SSEClient represents a connected SSE client with a buffered event channel.
// It uses sync.Once to ensure the channel is closed only once.
type SSEClient struct {
	Ch   chan SSEEvent
	once sync.Once
}

// Close safely closes the SSE client's event channel.
// It is safe to call multiple times; only the first call will close the channel.
func (c *SSEClient) Close() {
	c.once.Do(func() { close(c.Ch) })
}

// ConnectRequest represents the JSON payload for connecting to a router.
type ConnectRequest struct {
	RouterID string `json:"router_id"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// DisconnectRequest represents the JSON payload for disconnecting from a router.
type DisconnectRequest struct {
	RouterID string `json:"router_id"`
}

// RunRequest represents the JSON payload for executing arbitrary RouterOS commands.
type RunRequest struct {
	RouterID string   `json:"router_id"`
	Args     []string `json:"args"`
}

// JSONResponse is the standard API response structure for all endpoints.
type JSONResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}
