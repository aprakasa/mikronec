package types

import (
	"sync"

	"github.com/go-routeros/routeros/v3"
)

type SessionWrap struct {
	Conn     *routeros.Client
	Mu       sync.Mutex
	Host     string
	Username string
	Password string
}

type SSEEvent struct {
	ID    string
	Event string
	Data  []byte
}

type SSEClient struct {
	Ch   chan SSEEvent
	once sync.Once
}

func (c *SSEClient) Close() {
	c.once.Do(func() { close(c.Ch) })
}

type ConnectRequest struct {
	RouterID string `json:"router_id"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type DisconnectRequest struct {
	RouterID string `json:"router_id"`
}

type RunRequest struct {
	RouterID string   `json:"router_id"`
	Args     []string `json:"args"`
}

type JSONResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}
