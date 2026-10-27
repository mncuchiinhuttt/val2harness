package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"val2harness/internal/configfile"
)

// LocalBaseURL chooses a connectable local address for wildcard listeners.
func LocalBaseURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

const (
	DefaultUpstreamURL = "https://val.rmit.edu.au"
	DefaultListenAddr  = "127.0.0.1:8765"
)

type Config struct {
	Token                 string `json:"token"`
	UpstreamURL           string `json:"upstream_url"`
	ListenAddr            string `json:"listen_addr"`
	APIKey                string `json:"api_key,omitempty"`
	DaemonControlToken    string `json:"-"`
	DaemonControlIdentity string `json:"-"`
}

func DefaultConfig() *Config {
	return &Config{
		Token:       "",
		UpstreamURL: DefaultUpstreamURL,
		ListenAddr:  DefaultListenAddr,
	}
}

func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "val2harness")
	return dir, nil
}

func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	cfg := DefaultConfig()

	path, err := GetConfigPath()
	if err != nil {
		return cfg, err
	}

	// Provision against disk state, before transient environment overrides. The
	// shared file lock makes concurrent CLI/daemon first loads choose one key.
	err = configfile.Update(path, func(data []byte) ([]byte, error) {
		values, err := configfile.JSONObject(data)
		if err != nil {
			return nil, err
		}
		if data != nil {
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, err
			}
		}
		if cfg.APIKey != "" {
			return data, nil
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, err
		}
		cfg.APIKey = base64.RawURLEncoding.EncodeToString(secret)
		values["api_key"] = cfg.APIKey
		return json.MarshalIndent(values, "", "  ")
	})
	if err != nil {
		return nil, err
	}

	// Environment variable overrides
	if envToken := strings.TrimSpace(os.Getenv("VAL_RMIT_TOKEN")); envToken != "" {
		cfg.Token = envToken
	}
	if envUpstream := strings.TrimSpace(os.Getenv("VAL_RMIT_UPSTREAM")); envUpstream != "" {
		cfg.UpstreamURL = envUpstream
	}
	if envAddr := strings.TrimSpace(os.Getenv("VAL_RMIT_ADDR")); envAddr != "" {
		cfg.ListenAddr = envAddr
	}

	return cfg, nil
}

func (c *Config) Save() error {
	dir, err := GetConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path := filepath.Join(dir, "config.json")
	return configfile.Update(path, func(data []byte) ([]byte, error) {
		values, err := configfile.JSONObject(data)
		if err != nil {
			return nil, err
		}
		values["token"] = c.Token
		values["upstream_url"] = c.UpstreamURL
		values["listen_addr"] = c.ListenAddr
		values["api_key"] = c.APIKey
		return json.MarshalIndent(values, "", "  ")
	})
}
