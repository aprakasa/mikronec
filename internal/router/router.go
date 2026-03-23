// Package router manages RouterOS connections, session multiplexing, and SSE broadcasting.
// It provides connection pooling where multiple router_ids can share the same
// underlying connection when using identical credentials.
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aprakasa/mikronek/internal/normalize"
	"github.com/aprakasa/mikronek/internal/types"
	"github.com/go-routeros/routeros/v3"
)

// Manager handles router sessions, SSE clients, and polling lifecycle.
// It implements connection multiplexing where sessions are keyed by "host|user|pass"
// and can be shared across multiple router_ids.
type Manager struct {
	sessions   map[string]*types.SessionWrap
	routerMap  map[string]string
	sseHUB     map[string]map[*types.SSEClient]bool
	pollerStop map[string]chan struct{}
	mu         sync.RWMutex
}

// NewManager creates a new Manager with initialized maps for sessions,
// router mappings, SSE hubs, and poller stop channels.
func NewManager() *Manager {
	return &Manager{
		sessions:   make(map[string]*types.SessionWrap),
		routerMap:  make(map[string]string),
		sseHUB:     make(map[string]map[*types.SSEClient]bool),
		pollerStop: make(map[string]chan struct{}),
	}
}

// SessionExists returns the session wrap and existence boolean for a given session key.
// The key format is "host|user|pass".
func (m *Manager) SessionExists(key string) (*types.SessionWrap, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sw, ok := m.sessions[key]
	return sw, ok
}

// AddRouter registers a router_id to a session key mapping.
// This allows multiple router_ids to share the same underlying connection.
func (m *Manager) AddRouter(routerID, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routerMap[routerID] = key
}

// RemoveRouter removes the router_id mapping and returns the associated session key
// and whether the router was found.
func (m *Manager) RemoveRouter(routerID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.routerMap[routerID]
	if ok {
		delete(m.routerMap, routerID)
	}
	return key, ok
}

// IsKeyUsed checks if any router is currently using the given session key.
// This determines whether a session can be safely closed.
func (m *Manager) IsKeyUsed(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.routerMap {
		if v == key {
			return true
		}
	}
	return false
}

// ConnectRouter establishes a connection to a RouterOS device using the provided credentials.
// If a session already exists for the key, it validates the connection with a ping.
// If the existing connection is stale, it creates a new one.
func (m *Manager) ConnectRouter(ctx context.Context, key, host, user, pass string) error {
	m.mu.RLock()
	sw, exists := m.sessions[key]
	m.mu.RUnlock()

	if exists {
		sw.Mu.Lock()
		_, err := sw.Conn.Run("/ping", "=count=1")
		sw.Mu.Unlock()
		if err == nil {
			return nil
		}
	}

	conn, err := routeros.DialContext(ctx, normalize.EnsureAPIAddr(host), user, pass)
	if err != nil {
		return err
	}
	sw = &types.SessionWrap{Conn: conn, Host: host, Username: user, Password: pass}

	m.mu.Lock()
	m.sessions[key] = sw
	m.mu.Unlock()

	return nil
}

// GetConn retrieves a valid connection for the given router_id.
// It validates the connection with a ping and attempts reconnection if needed.
// Returns an error if the router is not connected or reconnection fails.
func (m *Manager) GetConn(ctx context.Context, routerID string) (*types.SessionWrap, error) {
	m.mu.RLock()
	key, ok := m.routerMap[routerID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("router %s not connected", routerID)
	}

	m.mu.RLock()
	sw := m.sessions[key]
	m.mu.RUnlock()
	if sw == nil {
		return nil, fmt.Errorf("router session missing for key %s", key)
	}

	sw.Mu.Lock()
	_, err := sw.Conn.Run("/ping", "=count=1")
	sw.Mu.Unlock()
	if err != nil {
		return nil, m.reconnect(ctx, key, sw.Host, sw.Username, sw.Password)
	}
	return sw, nil
}

// reconnect establishes a new connection to replace a stale one.
// It updates the session in the manager with the new connection.
func (m *Manager) reconnect(ctx context.Context, key, host, user, pass string) error {
	conn, err := routeros.DialContext(ctx, normalize.EnsureAPIAddr(host), user, pass)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.sessions[key] = &types.SessionWrap{Conn: conn, Host: host, Username: user, Password: pass}
	m.mu.Unlock()
	return nil
}

// CloseSession closes the RouterOS connection for the given session key
// and removes it from the manager.
func (m *Manager) CloseSession(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sw := m.sessions[key]; sw != nil {
		_ = sw.Conn.Close()
	}
	delete(m.sessions, key)
}

// StartPolling begins the polling loop for a router if not already running.
// The poller fetches hardware stats, hotspot active users, and PPP active users
// every 2 seconds and broadcasts to SSE clients.
func (m *Manager) StartPolling(routerID string) {
	m.mu.Lock()
	if _, running := m.pollerStop[routerID]; running {
		m.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	m.pollerStop[routerID] = stop
	m.mu.Unlock()

	log.Println("Start polling for:", routerID)

	go m.pollingLoop(routerID, stop)
}

// pollingLoop runs the periodic polling for a router until stopped.
// It cleans up the pollerStop entry when exiting.
func (m *Manager) pollingLoop(routerID string, stop chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer func() {
		ticker.Stop()
		m.mu.Lock()
		delete(m.pollerStop, routerID)
		m.mu.Unlock()
		log.Println("Polling stopped for:", routerID)
	}()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			m.poll(routerID)
		}
	}
}

// poll fetches data from the router and broadcasts it to connected SSE clients.
// It retrieves system resources, hotspot active users, and PPP active sessions.
func (m *Manager) poll(routerID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sw, err := m.GetConn(ctx, routerID)
	if err != nil {
		log.Printf("Poller for %s: connection error, retrying... (%v)", routerID, err)
		return
	}

	sw.Mu.Lock()
	hw, _ := sw.Conn.Run("/system/resource/print")
	hs, _ := sw.Conn.Run("/ip/hotspot/active/print")
	ppp, _ := sw.Conn.Run("/ppp/active/print")
	sw.Mu.Unlock()

	data := map[string]interface{}{
		"router_id":      routerID,
		"hardware":       normalize.Normalize(hw.Re),
		"hotspot_active": normalize.Normalize(hs.Re),
		"ppp_active":     normalize.Normalize(ppp.Re),
		"ts":             time.Now().Unix(),
	}
	b, _ := json.Marshal(data)

	id := fmt.Sprintf("%d", time.Now().UnixMilli())
	evt := types.SSEEvent{ID: id, Event: "poll", Data: b}

	m.mu.RLock()
	clients := make([]*types.SSEClient, 0, len(m.sseHUB[routerID]))
	for c := range m.sseHUB[routerID] {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.Ch <- evt:
		default:
		}
	}
}

// StopRouter stops the polling loop and closes all SSE clients for a router.
// It should be called when disconnecting a router.
func (m *Manager) StopRouter(routerID string) {
	m.mu.Lock()
	if stop, running := m.pollerStop[routerID]; running {
		close(stop)
	}
	if set, ok := m.sseHUB[routerID]; ok {
		for c := range set {
			c.Close()
		}
		delete(m.sseHUB, routerID)
	}
	m.mu.Unlock()
}

// AddSSEClient registers an SSE client for receiving updates from a router.
// It initializes the client set for the router if not present.
func (m *Manager) AddSSEClient(routerID string, client *types.SSEClient) {
	m.mu.Lock()
	if m.sseHUB[routerID] == nil {
		m.sseHUB[routerID] = map[*types.SSEClient]bool{}
	}
	m.sseHUB[routerID][client] = true
	m.mu.Unlock()
}

// RemoveSSEClient removes an SSE client from a router's broadcast list.
// If no clients remain, it automatically stops polling for that router.
func (m *Manager) RemoveSSEClient(routerID string, client *types.SSEClient) {
	m.mu.Lock()
	if set, ok := m.sseHUB[routerID]; ok {
		delete(set, client)
	}
	if len(m.sseHUB[routerID]) == 0 {
		if stop, ok := m.pollerStop[routerID]; ok {
			close(stop)
			log.Println("Auto stopped polling (no clients) for:", routerID)
		}
	}
	m.mu.Unlock()
}

// CloseAll gracefully shuts down all sessions, pollers, and SSE clients.
// It should be called during server shutdown.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for routerID, stop := range m.pollerStop {
		close(stop)
		delete(m.pollerStop, routerID)
	}

	for routerID, set := range m.sseHUB {
		for c := range set {
			c.Close()
		}
		delete(m.sseHUB, routerID)
	}

	for key, sw := range m.sessions {
		_ = sw.Conn.Close()
		delete(m.sessions, key)
	}

	log.Println("All sessions, pollers, and SSE clients closed")
}

// IsRouterConnected checks if a router_id is currently registered in the manager.
func (m *Manager) IsRouterConnected(routerID string) bool {
	m.mu.RLock()
	_, connected := m.routerMap[routerID]
	m.mu.RUnlock()
	return connected
}
