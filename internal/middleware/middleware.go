// Package middleware provides HTTP middleware for authentication and CORS handling.
package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Config holds the middleware configuration including API key and allowed CORS origins.
type Config struct {
	APIKey         string
	AllowedOrigins map[string]bool
}

// AuthMiddleware returns a middleware that validates the X-API-Key header
// against the configured API key. Requests with invalid or missing keys
// receive a 401 Unauthorized response.
func AuthMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientKey := r.Header.Get("X-API-Key")
			if clientKey != apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware returns a middleware that handles Cross-Origin Resource Sharing.
// It sets appropriate CORS headers for requests from allowed origins and
// handles OPTIONS preflight requests.
func CORSMiddleware(allowedOrigins map[string]bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestOrigin := r.Header.Get("Origin")
			if _, ok := allowedOrigins[requestOrigin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// LoadEnvConfig loads middleware configuration from environment variables.
// It reads API_KEY (required) and ALLOWED_ORIGINS (comma-separated, required).
// Returns an error if any required variable is missing.
func LoadEnvConfig() (*Config, error) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("API_KEY environment variable is required")
	}

	originsStr := os.Getenv("ALLOWED_ORIGINS")
	if originsStr == "" {
		return nil, fmt.Errorf("ALLOWED_ORIGINS environment variable is required (comma-separated)")
	}

	allowedOrigins := make(map[string]bool)
	for _, origin := range strings.Split(originsStr, ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed != "" {
			allowedOrigins[trimmed] = true
		}
	}

	return &Config{
		APIKey:         apiKey,
		AllowedOrigins: allowedOrigins,
	}, nil
}
