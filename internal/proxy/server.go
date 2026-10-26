package proxy

import (
	"context"
	"net/http"
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
	// CORS Headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, Origin, User-Agent")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Verify local API key if configured
	if s.cfg.APIKey != "" {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + s.cfg.APIKey
		if auth != expected {
			http.Error(w, `{"error":{"message":"Unauthorized"}}`, http.StatusUnauthorized)
			return
		}
	}

	s.mux.ServeHTTP(w, r)
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
		WriteTimeout: 0, // No write timeout for long SSE streams
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
