package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"val2harness/internal/config"
)

// DashboardURL obtains a one-use sign-in ticket, never putting the permanent key
// into a browser URL or daemon log. The caller decides whether to start a server.
func DashboardURL(cfg *config.Config) (string, error) {
	base := config.LocalBaseURL(cfg.ListenAddr)
	req, err := http.NewRequest(http.MethodPost, base+"/api/local/session-ticket", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("gateway sign-in failed (HTTP %d); check the local key or restart the gateway", response.StatusCode)
	}
	var result struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Ticket == "" {
		return "", fmt.Errorf("gateway returned no sign-in ticket")
	}
	return base + "/#session=" + result.Ticket, nil
}
