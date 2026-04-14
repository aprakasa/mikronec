// Package main is the entry point for the Mikronec server.
// Mikronec is a high-performance Go backend server that acts as a bridge
// to manage and monitor multiple MikroTik routers through a single secure API.
//
// @title Mikronec API
// @version 1.0
// @description A high-performance Go backend for managing and monitoring MikroTik routers.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email aprakasa@example.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @description API Key for authentication
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/aprakasa/mikronek/docs"
	"github.com/aprakasa/mikronek/internal/handler"
	"github.com/aprakasa/mikronek/internal/middleware"
	"github.com/aprakasa/mikronek/internal/router"
	httpSwagger "github.com/swaggo/http-swagger"
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

	mux.HandleFunc("GET /swagger/", httpSwagger.WrapHandler)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		handler.JSONOK(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
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

	log.Println("MikroHot Connector running at :" + port) // nolint:gosec

	srv := &http.Server{Addr: ":" + port, Handler: h, ReadHeaderTimeout: 10 * time.Second}

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
