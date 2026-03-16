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

type Manager struct {
	sessions   map[string]*types.SessionWrap
	routerMap  map[string]string
	sseHUB     map[string]map[*types.SSEClient]bool
	pollerStop map[string]chan struct{}
	mu         sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		sessions:   make(map[string]*types.SessionWrap),
		routerMap:  make(map[string]string),
		sseHUB:     make(map[string]map[*types.SSEClient]bool),
		pollerStop: make(map[string]chan struct{}),
	}
}

func (m *Manager) SessionExists(key string) (*types.SessionWrap, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sw, ok := m.sessions[key]
	return sw, ok
}

func (m *Manager) AddRouter(routerID, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routerMap[routerID] = key
}

func (m *Manager) RemoveRouter(routerID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.routerMap[routerID]
	if ok {
		delete(m.routerMap, routerID)
	}
	return key, ok
}

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

func (m *Manager) CloseSession(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sw := m.sessions[key]; sw != nil {
		_ = sw.Conn.Close()
	}
	delete(m.sessions, key)
}

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

func (m *Manager) AddSSEClient(routerID string, client *types.SSEClient) {
	m.mu.Lock()
	if m.sseHUB[routerID] == nil {
		m.sseHUB[routerID] = map[*types.SSEClient]bool{}
	}
	m.sseHUB[routerID][client] = true
	m.mu.Unlock()
}

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

func (m *Manager) IsRouterConnected(routerID string) bool {
	m.mu.RLock()
	_, connected := m.routerMap[routerID]
	m.mu.RUnlock()
	return connected
}
