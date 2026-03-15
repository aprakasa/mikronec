package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddleware(t *testing.T) {
	apiKey := "test-api-key"

	tests := []struct {
		name       string
		apiKey     string
		wantStatus int
	}{
		{"valid key", "test-api-key", http.StatusOK},
		{"invalid key", "wrong-key", http.StatusUnauthorized},
		{"missing key", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := AuthMiddleware(apiKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	allowedOrigins := map[string]bool{
		"http://localhost:3000": true,
	}

	handler := CORSMiddleware(allowedOrigins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "http://localhost:3000")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
			t.Errorf("got Access-Control-Allow-Origin %s, want http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("disallowed origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", "http://evil.com")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("got Access-Control-Allow-Origin %s, want empty", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("OPTIONS preflight", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/", nil)
		req.Header.Set("Origin", "http://localhost:3000")

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", w.Code, http.StatusOK)
		}
	})
}

func TestLoadEnvConfig(t *testing.T) {
	t.Setenv("API_KEY", "test-key")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:8080")

	cfg, err := LoadEnvConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIKey != "test-key" {
		t.Errorf("got APIKey %s, want test-key", cfg.APIKey)
	}

	if !cfg.AllowedOrigins["http://localhost:3000"] {
		t.Error("expected localhost:3000 to be allowed")
	}

	if !cfg.AllowedOrigins["http://localhost:8080"] {
		t.Error("expected localhost:8080 to be allowed")
	}
}

func TestLoadEnvConfigMissingAPIKey(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:3000")

	_, err := LoadEnvConfig()
	if err == nil {
		t.Error("expected error for missing API_KEY")
	}
}

func TestLoadEnvConfigMissingOrigins(t *testing.T) {
	t.Setenv("API_KEY", "test-key")

	_, err := LoadEnvConfig()
	if err == nil {
		t.Error("expected error for missing ALLOWED_ORIGINS")
	}
}
