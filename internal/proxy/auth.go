package proxy

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type credential struct {
	purpose string
	expires time.Time
}
type authState struct {
	mu       sync.Mutex
	tickets  map[string]credential
	sessions map[string]credential
	clock    func() time.Time
}

func newAuthState() *authState {
	return &authState{tickets: map[string]credential{}, sessions: map[string]credential{}, clock: time.Now}
}
func randomCredential() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (a *authState) issue(purpose string, session bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.clock()
	for key, value := range a.tickets {
		if !now.Before(value.expires) {
			delete(a.tickets, key)
		}
	}
	for key, value := range a.sessions {
		if !now.Before(value.expires) {
			delete(a.sessions, key)
		}
	}
	entries := a.tickets
	ttl := 5 * time.Minute
	if session {
		entries = a.sessions
		ttl = 8 * time.Hour
	}
	if len(entries) >= 1024 {
		return "", fmt.Errorf("too many active local sign-ins")
	}
	key, err := randomCredential()
	if err != nil {
		return "", err
	}
	entries[key] = credential{purpose, now.Add(ttl)}
	return key, nil
}
func (a *authState) consume(key, purpose string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.tickets[key]
	if !ok || v.purpose != purpose {
		return false
	}
	delete(a.tickets, key)
	return a.clock().Before(v.expires)
}
func (a *authState) session(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.sessions[key]
	if !ok {
		return false
	}
	if !a.clock().Before(v.expires) {
		delete(a.sessions, key)
		return false
	}
	return true
}

func authError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": message}})
}
func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) validHost(r *http.Request) bool {
	configured, port, err := net.SplitHostPort(s.cfg.ListenAddr)
	if err != nil {
		return false
	}
	host, requestPort, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
		requestPort = "80"
		if r.TLS != nil {
			requestPort = "443"
		}
	}
	if port == "0" {
		if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
			_, port, _ = net.SplitHostPort(addr.String())
		}
	}
	if port != requestPort {
		return false
	}
	configured = strings.ToLower(configured)
	host = strings.ToLower(host)
	if configured == "" || configured == "0.0.0.0" || configured == "::" {
		return host == "localhost" || net.ParseIP(host) != nil
	}
	ip := net.ParseIP(configured)
	if configured == "localhost" || (ip != nil && ip.IsLoopback()) {
		candidate := net.ParseIP(host)
		return host == "localhost" || (candidate != nil && candidate.IsLoopback())
	}
	return host == configured
}
func ownOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
func (s *Server) upstreamOrigin() string {
	u, err := url.Parse(s.cfg.UpstreamURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
func (s *Server) authorized(r *http.Request) bool {
	// Never let an accidentally uninitialized server become unauthenticated.
	if s.cfg.APIKey == "" {
		return false
	}
	key := ""
	auth := r.Header.Get("Authorization")
	if auth != "" {
		if !strings.HasPrefix(auth, "Bearer ") {
			return false
		}
		key = strings.TrimPrefix(auth, "Bearer ")
	} else {
		key = r.Header.Get("x-api-key")
	}
	if key != "" && s.cfg.DaemonControlToken != "" && subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.DaemonControlToken)) == 1 {
		return false
	}
	return key != "" && (subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.APIKey)) == 1 || s.auth.session(key))
}

// guard owns the shared model and management boundary, including route aliases.
func (s *Server) guard(w http.ResponseWriter, r *http.Request) bool {
	if !s.validHost(r) {
		authError(w, 403, "Unexpected gateway host")
		return false
	}
	origin := r.Header.Get("Origin")
	importRoute := r.URL.Path == "/api/local/import-token"
	allowed := ownOrigin(r)
	if importRoute {
		allowed = s.upstreamOrigin()
	}
	if origin != "" && origin != allowed {
		authError(w, 403, "This browser origin is not permitted")
		return false
	}
	if importRoute && (origin == "" || allowed == "") {
		authError(w, 403, "Token import requires the configured Val origin")
		return false
	}
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Vary", "Origin")
	}
	// A daemon capability authorizes this one literal leaf only. It cannot
	// become an owner key, browser session, inference key, or route prefix.
	if r.URL.Path == "/api/local/daemon-control" {
		if r.URL.EscapedPath() != "/api/local/daemon-control" {
			authError(w, 401, "Daemon control requires its literal route")
			return false
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			authError(w, 405, "Daemon control requires GET or POST")
			return false
		}
		expected := s.cfg.DaemonControlToken
		authorization := r.Header.Get("Authorization")
		if expected == "" || s.cfg.DaemonControlIdentity == "" || !strings.HasPrefix(authorization, "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(authorization, "Bearer ")), []byte(expected)) != 1 {
			authError(w, 401, "Invalid daemon control capability")
			return false
		}
		w.Header().Set("Cache-Control", "no-store")
		return true
	}
	if r.Method == http.MethodOptions {
		methods := "GET, POST, OPTIONS"
		headers := "Authorization, Content-Type, x-api-key"
		if importRoute {
			methods = "POST, OPTIONS"
			headers = "Content-Type, X-Val-Import"
		}
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", headers)
		w.WriteHeader(204)
		return false
	}
	if importRoute {
		if r.Method != http.MethodPost {
			authError(w, 405, "Token import requires POST")
			return false
		}
		if !s.auth.consume(r.Header.Get("X-Val-Import"), "val-import") {
			authError(w, 401, "Token import link expired or already used")
			return false
		}
		return true
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && (r.URL.Path == "/" || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/")) {
		return true
	}
	if r.URL.Path == "/api/local/session" {
		s.exchangeSession(w, r)
		return false
	}
	if !s.authorized(r) {
		authError(w, 401, "Unlock the local gateway or provide its connection key")
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	return true
}
func (s *Server) exchangeSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		authError(w, 405, "Sign-in requires POST")
		return
	}
	var payload struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&payload); err != nil {
		authError(w, 400, "Invalid sign-in request")
		return
	}
	if !s.auth.consume(payload.Ticket, "dashboard") {
		authError(w, 401, "Sign-in link expired or already used")
		return
	}
	token, err := s.auth.issue("dashboard", true)
	if err != nil {
		authError(w, 503, "Could not create a local session")
		return
	}
	jsonReply(w, map[string]string{"token": token})
}
func (s *Server) registerAuthRoutes() {
	s.mux.HandleFunc("/api/local/session-ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			authError(w, 405, "Sign-in link requires POST")
			return
		}
		ticket, err := s.auth.issue("dashboard", false)
		if err != nil {
			authError(w, 503, "Could not create a sign-in link")
			return
		}
		jsonReply(w, map[string]string{"ticket": ticket})
	})
	s.mux.HandleFunc("/api/local/import-ticket", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			authError(w, 405, "Token import link requires POST")
			return
		}
		ticket, err := s.auth.issue("val-import", false)
		if err != nil {
			authError(w, 503, "Could not create a token import link")
			return
		}
		jsonReply(w, map[string]string{"ticket": ticket})
	})
}
