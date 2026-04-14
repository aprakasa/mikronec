// Package handler provides HTTP handlers for the Mikronec API endpoints.
// This file contains the Server-Sent Events (SSE) handler for real-time streaming.
package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/aprakasa/mikronek/internal/router"
	"github.com/aprakasa/mikronek/internal/types"
)

// sseRetryMs is the retry interval in milliseconds sent to SSE clients
// for automatic reconnection on connection loss.
const sseRetryMs = 3000

// HandleSSE handles GET /sse/{routerID} requests for Server-Sent Events streaming.
// It streams real-time router data including hardware stats, hotspot active users,
// and PPP active sessions. The connection stays open until the client disconnects.
// Supports reconnection via Last-Event-ID header.
//
// @Summary Subscribe to router SSE stream
// @Description Stream real-time router data (hardware stats, hotspot users, PPP sessions) via Server-Sent Events.
// @Tags streaming
// @Produce text/event-stream
// @Param X-API-Key header string true "API Key for authentication"
// @Param routerID path string true "Router ID to subscribe to"
// @Param Last-Event-ID header string false "Last event ID for reconnection"
// @Success 200 "SSE stream started"
// @Failure 400 {object} types.JSONResponse "Missing router or router not connected"
// @Failure 401 {object} types.JSONResponse "Unauthorized"
// @Router /sse/{routerID} [get]
func HandleSSE(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
	routerID := r.PathValue("routerID")
	if routerID == "" {
		JSONErr(w, "missing router", http.StatusBadRequest)
		return
	}

	if !rm.IsRouterConnected(routerID) {
		JSONErr(w, "router not connected", http.StatusBadRequest)
		return
	}

	lastEventID := r.Header.Get("Last-Event-ID")

	client := &types.SSEClient{Ch: make(chan types.SSEEvent, 10)}

	rm.AddSSEClient(routerID, client)
	rm.StartPolling(routerID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)

	_, _ = fmt.Fprintf(w, "retry: %d\n\n", sseRetryMs)
	flusher.Flush()

	if lastEventID != "" {
		if _, err := strconv.ParseInt(lastEventID, 10, 64); err == nil {
			_, _ = fmt.Fprintf(w, "event: reconnect\nid: %s\ndata: {\"reconnected\":true,\"last_event_id\":\"%s\"}\n\n", lastEventID, lastEventID) // nolint:gosec
			flusher.Flush()
		}
	}

	done := r.Context().Done()
	go func() {
		<-done
		rm.RemoveSSEClient(routerID, client)
		client.Close()
	}()

	for evt := range client.Ch {
		w.Write([]byte("event: " + evt.Event + "\n"))       // nolint:errcheck,gosec
		w.Write([]byte("id: " + evt.ID + "\n"))             // nolint:errcheck,gosec
		w.Write([]byte("data: " + string(evt.Data) + "\n")) // nolint:errcheck,gosec
		w.Write([]byte("\n"))                               // nolint:errcheck,gosec
		flusher.Flush()
	}
}
