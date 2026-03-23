// Package handler provides HTTP handlers for the Mikronec API endpoints.
// It includes handlers for connecting, disconnecting, system info, and command execution.
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

// JSONErr logs an internal error message and writes a JSON error response.
// The public error message is sanitized based on the HTTP status code.
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

// JSONOK writes a successful JSON response with the provided data.
func JSONOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types.JSONResponse{Success: true, Data: data})
}

// HandleConnect handles POST /connect requests to establish a connection to a router.
// It accepts router_id, host, username, and password in the request body.
// The connection is pooled and can be shared across multiple router_ids with the same credentials.
// It also starts the polling loop for real-time data.
//
// @Summary Connect to a MikroTik router
// @Description Establish a connection to a MikroTik router. Connections are pooled and can be reused.
// @Tags connection
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param request body types.ConnectRequest true "Connection details"
// @Success 200 {object} types.JSONResponse{data=map[string]interface{}} "Connected successfully"
// @Failure 400 {object} types.JSONResponse "Invalid request"
// @Failure 401 {object} types.JSONResponse "Unauthorized"
// @Failure 500 {object} types.JSONResponse "Connection failed"
// @Router /connect [post]
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

// HandleDisconnect handles POST /disconnect requests to close a router connection.
// It stops polling, closes SSE clients, and optionally closes the underlying session
// if no other routers are using it.
//
// @Summary Disconnect from a MikroTik router
// @Description Close the connection to a MikroTik router. Stops polling and SSE streams.
// @Tags connection
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param request body types.DisconnectRequest true "Disconnect details"
// @Success 200 {object} types.JSONResponse{data=map[string]interface{}} "Disconnected successfully"
// @Failure 400 {object} types.JSONResponse "Invalid request"
// @Failure 401 {object} types.JSONResponse "Unauthorized"
// @Router /disconnect [post]
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

// HandleSystemInfo handles GET /system-info requests to retrieve router identity
// and hardware information. Requires the "router" query parameter.
//
// @Summary Get router system information
// @Description Retrieve identity, version, architecture, model, and serial number from the router.
// @Tags system
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param router query string true "Router ID to query"
// @Success 200 {object} types.JSONResponse{data=map[string]string} "System information"
// @Failure 400 {object} types.JSONResponse "Missing router parameter"
// @Failure 401 {object} types.JSONResponse "Unauthorized"
// @Failure 500 {object} types.JSONResponse "Connection error"
// @Router /system-info [get]
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

// HandleRun handles POST /run requests to execute arbitrary RouterOS commands.
// This is a powerful endpoint that should be used with caution.
// It accepts router_id and args (command and arguments) in the request body.
//
// @Summary Execute a RouterOS command
// @Description Execute arbitrary RouterOS commands on a connected router. Use with caution.
// @Tags commands
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param request body types.RunRequest true "Command to execute"
// @Success 200 {object} types.JSONResponse{data=[]map[string]interface{}} "Command result"
// @Failure 400 {object} types.JSONResponse "Invalid request"
// @Failure 401 {object} types.JSONResponse "Unauthorized"
// @Failure 500 {object} types.JSONResponse "Command execution failed"
// @Router /run [post]
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
