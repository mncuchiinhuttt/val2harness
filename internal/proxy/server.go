package proxy

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

type Server struct {
	cfg        *config.Config
	client     *valclient.Client
	httpServer *http.Server
	mux        *http.ServeMux
	mu         sync.RWMutex
}

func NewServer(cfg *config.Config, client *valclient.Client) *Server {
	s := &Server{
		cfg:    cfg,
		client: client,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) UpdateToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Token = token
	s.client.SetToken(token)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/messages", s.handleMessages)
	s.mux.HandleFunc("/messages", s.handleMessages)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	path := r.URL.Path

	// Security response headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	// Protect management API: loopback callers & local origins only
	if strings.HasPrefix(path, "/api/local/") {
		if !isLoopbackAddr(r.RemoteAddr) {
			http.Error(w, `{"error":{"message":"Forbidden: loopback access only"}}`, http.StatusForbidden)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			http.Error(w, `{"error":{"message":"Forbidden: cross-site request"}}`, http.StatusForbidden)
			return
		}
		if origin != "" {
			if !isAllowedLocalOrigin(origin) {
				http.Error(w, `{"error":{"message":"Forbidden: untrusted origin"}}`, http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	} else {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Origin, User-Agent, x-api-key, anthropic-version")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify local API key if configured
	if s.cfg.APIKey != "" && !strings.HasPrefix(path, "/static/") && path != "/" {
		auth := r.Header.Get("Authorization")
		key := r.Header.Get("x-api-key")
		if !verifyLocalAuth(auth, key, s.cfg.APIKey) {
			http.Error(w, `{"error":{"message":"Unauthorized"}}`, http.StatusUnauthorized)
			return
		}
	}

	s.mux.ServeHTTP(w, r)
}

func isAllowedLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func verifyLocalAuth(authHeader, apiKeyHeader, expectedKey string) bool {
	expBytes := []byte(expectedKey)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), expBytes) == 1 {
			return true
		}
	}
	if apiKeyHeader != "" && subtle.ConstantTimeCompare([]byte(apiKeyHeader), expBytes) == 1 {
		return true
	}
	return false
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","upstream":"` + s.cfg.UpstreamURL + `"}`))
}

func (s *Server) Start() error {
	s.httpServer = &http.Server{
		Addr:         s.cfg.ListenAddr,
		Handler:      s,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 0,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) GetMux() *http.ServeMux {
	return s.mux
}
