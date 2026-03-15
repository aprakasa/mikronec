package handler

import (
	"fmt"
	"net/http"

	"github.com/aprakasa/mikronek/internal/router"
	"github.com/aprakasa/mikronek/internal/types"
)

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

	client := &types.SSEClient{Ch: make(chan []byte, 10)}

	rm.AddSSEClient(routerID, client)
	rm.StartPolling(routerID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	done := r.Context().Done()
	go func() {
		<-done
		rm.RemoveSSEClient(routerID, client)
		client.Close()
	}()

	for msg := range client.Ch {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}
}
