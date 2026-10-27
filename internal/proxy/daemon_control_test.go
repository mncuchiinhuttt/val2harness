package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDaemonControlScopeAndDenials(t *testing.T) {
	s := authFixture(t, "local-owner")
	s.cfg.DaemonControlToken = "synthetic-daemon-capability"
	s.cfg.DaemonControlIdentity = "synthetic-process-identity"
	call := func(method, path, token string, extra map[string]string) int {
		headers := map[string]string{}
		if token != "" {
			headers["Authorization"] = "Bearer " + token
		}
		for k, v := range extra {
			headers[k] = v
		}
		return authCall(s, method, path, "", headers).Code
	}
	if got := call("GET", "/api/local/daemon-control", s.cfg.DaemonControlToken, nil); got != 200 {
		t.Fatalf("control handshake HTTP %d", got)
	}
	for _, tc := range []struct {
		method, path, token string
		headers             map[string]string
		want                int
	}{
		{"GET", "/api/local/daemon-control", "", nil, 401},
		{"GET", "/api/local/daemon-control", "wrong", nil, 401},
		{"GET", "/api/local/daemon-control", "local-owner", nil, 401},
		{"GET", "/api/local/daemon-control", "", map[string]string{"Cookie": "token=synthetic-daemon-capability"}, 401},
		{"GET", "/api/local/daemon-control?token=synthetic-daemon-capability", "", nil, 401},
		{"GET", "/api/local/daemon-control", "", map[string]string{"x-api-key": "synthetic-daemon-capability"}, 401},
		{"HEAD", "/api/local/daemon-control", s.cfg.DaemonControlToken, nil, 405},
		{"PUT", "/api/local/daemon-control", s.cfg.DaemonControlToken, nil, 405},
		{"DELETE", "/api/local/daemon-control", s.cfg.DaemonControlToken, nil, 405},
		{"OPTIONS", "/api/local/daemon-control", s.cfg.DaemonControlToken, nil, 405},
		{"GET", "/api/local/daemon-control/", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/api/local/daemon-control/extra", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/api/local/%64aemon-control", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/api/local/%64aemon-control", "local-owner", nil, 401},
		{"GET", "/api/local/daemon-control", "", map[string]string{"Authorization": "Basic synthetic-daemon-capability"}, 401},
		{"GET", "/api/local/daemon-control", s.cfg.DaemonControlToken, map[string]string{"Origin": "https://untrusted.invalid"}, 403},
		{"GET", "/v1/models", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/models", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/v1/chat/completions", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/chat/completions", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/v1/messages", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/messages", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/api/local/session-ticket", s.cfg.DaemonControlToken, nil, 401},
		{"POST", "/api/local/import-ticket", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/api/status", s.cfg.DaemonControlToken, nil, 401},
		{"GET", "/api/local/protected", s.cfg.DaemonControlToken, nil, 401},
	} {
		t.Run(tc.method+tc.path+tc.token, func(t *testing.T) {
			if got := call(tc.method, tc.path, tc.token, tc.headers); got != tc.want {
				t.Errorf("got HTTP %d, want %d", got, tc.want)
			}
		})
	}
	response := authCall(s, "GET", "/api/local/daemon-control", "", map[string]string{"Authorization": "Bearer " + s.cfg.DaemonControlToken})
	var result struct {
		PID      int    `json:"pid"`
		Identity string `json:"identity"`
		Ready    bool   `json:"ready"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.PID <= 0 || result.Identity != "synthetic-process-identity" || !result.Ready {
		t.Fatalf("bad handshake: %+v", result)
	}
	// Empty configuration must never turn empty credentials into authority.
	s.cfg.DaemonControlToken = ""
	if got := call("GET", "/api/local/daemon-control", "", nil); got != 401 {
		t.Fatalf("empty control token HTTP %d", got)
	}
}

func TestDaemonCapabilityCannotDoubleAsOwnerKey(t *testing.T) {
	s := authFixture(t, "synthetic-same-key")
	s.cfg.DaemonControlToken = "synthetic-same-key"
	s.cfg.DaemonControlIdentity = "synthetic-identity"
	if got := authCall(s, "GET", "/api/local/protected", "", map[string]string{"Authorization": "Bearer synthetic-same-key"}).Code; got != 401 {
		t.Fatalf("control capability became owner authority: HTTP %d", got)
	}
}

func TestDaemonControlRejectsHostAndOnlyPOSTRequestsStop(t *testing.T) {
	s := authFixture(t, "local-owner")
	s.cfg.DaemonControlToken = "synthetic-daemon-capability"
	s.cfg.DaemonControlIdentity = "synthetic-process-identity"
	request := httptest.NewRequest("POST", "http://attacker.invalid:8765/api/local/daemon-control", nil)
	request.Header.Set("Authorization", "Bearer "+s.cfg.DaemonControlToken)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("foreign Host HTTP %d", response.Code)
	}
	select {
	case <-s.DaemonStopRequested():
		t.Fatal("foreign host stopped server")
	default:
	}
	if got := authCall(s, http.MethodPost, "/api/local/daemon-control", "", map[string]string{"Authorization": "Bearer wrong"}).Code; got != 401 {
		t.Fatalf("unauthenticated shutdown HTTP %d", got)
	}
	select {
	case <-s.DaemonStopRequested():
		t.Fatal("invalid token stopped server")
	default:
	}
	if got := authCall(s, http.MethodPost, "/api/local/daemon-control", "", map[string]string{"Authorization": "Bearer " + s.cfg.DaemonControlToken}).Code; got != 200 {
		t.Fatalf("authenticated shutdown HTTP %d", got)
	}
	select {
	case <-s.DaemonStopRequested():
	default:
		t.Fatal("authenticated POST did not request shutdown")
	}
}
