package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/aprakasa/mikronek/internal/router"
	"github.com/aprakasa/mikronek/internal/types"
)

const sseRetryMs = 3000

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
			_, _ = fmt.Fprintf(w, "event: reconnect\nid: %s\ndata: {\"reconnected\":true,\"last_event_id\":\"%s\"}\n\n", lastEventID, lastEventID)
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
		w.Write([]byte("event: " + evt.Event + "\n"))
		w.Write([]byte("id: " + evt.ID + "\n"))
		w.Write([]byte("data: " + string(evt.Data) + "\n"))
		w.Write([]byte("\n"))
		flusher.Flush()
	}
}
