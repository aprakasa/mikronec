package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aprakasa/mikronek/internal/handler"
	"github.com/aprakasa/mikronek/internal/middleware"
	"github.com/aprakasa/mikronek/internal/router"
)

func main() {
	cfg, err := middleware.LoadEnvConfig()
	if err != nil {
		log.Fatal("FATAL:", err)
	}

	for origin := range cfg.AllowedOrigins {
		log.Println("Allowing Origin:", origin)
	}

	rm := router.NewManager()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		handler.JSONOK(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		handler.JSONOK(w, map[string]string{"message": "ok"})
	})
	mux.HandleFunc("POST /connect", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleConnect(w, r, rm)
	})
	mux.HandleFunc("POST /disconnect", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleDisconnect(w, r, rm)
	})
	mux.HandleFunc("GET /system-info", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleSystemInfo(w, r, rm)
	})
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleRun(w, r, rm)
	})
	mux.HandleFunc("GET /sse/{branchID}", func(w http.ResponseWriter, r *http.Request) {
		handler.HandleSSE(w, r, rm)
	})

	var h http.Handler = mux
	h = middleware.AuthMiddleware(cfg.APIKey)(h)
	h = middleware.CORSMiddleware(cfg.AllowedOrigins)(h)

	log.Println("MikroHot Connector running at :" + port)

	srv := &http.Server{Addr: ":" + port, Handler: h}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("Received %v, shutting down gracefully...", sig)

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go rm.CloseAll()

	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
