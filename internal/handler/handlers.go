package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/aprakasa/mikronek/internal/normalize"
	"github.com/aprakasa/mikronek/internal/router"
	"github.com/aprakasa/mikronek/internal/types"
	"github.com/go-routeros/routeros/v3"
)

func JSONErr(w http.ResponseWriter, internalMsg string, code int) {
	log.Printf("Internal error (code %d): %s", code, internalMsg)

	var publicMsg string
	switch code {
	case http.StatusBadRequest:
		publicMsg = "Bad Request"
	case http.StatusUnauthorized:
		publicMsg = "Unauthorized"
	case http.StatusInternalServerError:
		publicMsg = "Internal Server Error"
	default:
		publicMsg = "An error occurred"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(types.JSONResponse{Success: false, Error: publicMsg})
}

func JSONOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types.JSONResponse{Success: true, Data: data})
}

func HandleConnect(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
	var body types.ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONErr(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if body.RouterID == "" || body.Host == "" || body.Username == "" {
		JSONErr(w, "router_id, host, username are required", http.StatusBadRequest)
		return
	}

	key := body.Host + "|" + body.Username + "|" + body.Password

	_, existed := rm.SessionExists(key)

	rm.AddRouter(body.RouterID, key)

	if err := rm.ConnectRouter(r.Context(), key, body.Host, body.Username, body.Password); err != nil {
		JSONErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rm.StartPolling(body.RouterID)

	JSONOK(w, map[string]interface{}{
		"router_id": body.RouterID,
		"connected": true,
		"shared":    true,
		"reused":    existed,
	})
}

func HandleDisconnect(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
	var body types.DisconnectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONErr(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if body.RouterID == "" {
		JSONErr(w, "router_id is required", http.StatusBadRequest)
		return
	}

	key, ok := rm.RemoveRouter(body.RouterID)
	if !ok {
		JSONOK(w, map[string]interface{}{"router_id": body.RouterID, "disconnected": false, "reason": "not connected"})
		return
	}

	rm.StopRouter(body.RouterID)

	stillUsed := rm.IsKeyUsed(key)
	if !stillUsed {
		rm.CloseSession(key)
	}

	JSONOK(w, map[string]interface{}{"router_id": body.RouterID, "disconnected": true, "router_closed": !stillUsed})
}

func HandleSystemInfo(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
	router := r.URL.Query().Get("router")
	if router == "" {
		JSONErr(w, "missing query param: router", http.StatusBadRequest)
		return
	}

	sw, err := rm.GetConn(r.Context(), router)
	if err != nil {
		JSONErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sw.Mu.Lock()
	idn, _ := sw.Conn.Run("/system/identity/print")
	sys, _ := sw.Conn.Run("/system/resource/print")
	rb, _ := sw.Conn.Run("/system/routerboard/print")
	sw.Mu.Unlock()

	get := func(rep *routeros.Reply, k string) string {
		if rep == nil || len(rep.Re) == 0 {
			return ""
		}
		return rep.Re[0].Map[k]
	}

	out := map[string]string{
		"identity": get(idn, "name"),
		"version":  get(sys, "version"),
		"arch":     get(sys, "architecture-name"),
		"model":    get(rb, "model"),
		"serial":   get(rb, "serial-number"),
	}
	JSONOK(w, out)
}

func HandleRun(w http.ResponseWriter, r *http.Request, rm *router.Manager) {
	var body types.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONErr(w, "bad json body", http.StatusBadRequest)
		return
	}
	if body.RouterID == "" || len(body.Args) == 0 {
		JSONErr(w, "router_id and args are required", http.StatusBadRequest)
		return
	}

	sw, err := rm.GetConn(r.Context(), body.RouterID)
	if err != nil {
		JSONErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sw.Mu.Lock()
	reply, err := sw.Conn.RunArgs(body.Args)
	sw.Mu.Unlock()
	if err != nil {
		JSONErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	JSONOK(w, normalize.Normalize(reply.Re))
}
