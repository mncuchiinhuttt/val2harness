package proxy

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

type Server struct {
	cfg            *config.Config
	client         *valclient.Client
	httpServer     *http.Server
	mux            *http.ServeMux
	auth           *authState
	daemonStop     chan struct{}
	daemonStopOnce sync.Once
}

func NewServer(cfg *config.Config, client *valclient.Client) *Server {
	s := &Server{
		cfg:        cfg,
		client:     client,
		mux:        http.NewServeMux(),
		auth:       newAuthState(),
		daemonStop: make(chan struct{}),
	}
	s.registerRoutes()
	s.registerAuthRoutes()
	return s
}

func (s *Server) UpdateToken(token string) {
	// The live client owns mutable credentials. Startup configuration is immutable.
	s.client.SetToken(token)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/api/local/daemon-control", func(w http.ResponseWriter, r *http.Request) {
		jsonReply(w, map[string]any{"pid": os.Getpid(), "identity": s.cfg.DaemonControlIdentity, "ready": true})
		if r.Method == http.MethodPost {
			s.daemonStopOnce.Do(func() { close(s.daemonStop) })
		}
	})
	s.mux.HandleFunc("/v1/models", s.handleModels)
	s.mux.HandleFunc("/models", s.handleModels)
	s.mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/chat/completions", s.handleChatCompletions)
	s.mux.HandleFunc("/v1/messages", s.handleMessages)
	s.mux.HandleFunc("/messages", s.handleMessages)
}

// DaemonStopRequested closes only after the narrowly scoped authenticated
// control route requests graceful shutdown. Foreground servers remain closed
// to this capability because their runtime control token is empty.
func (s *Server) DaemonStopRequested() <-chan struct{} { return s.daemonStop }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.guard(w, r) {
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
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
