package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultUpstreamURL = "https://val.rmit.edu.au"
	DefaultListenAddr  = "127.0.0.1:8765"
)

type Config struct {
	Token       string `json:"token"`
	UpstreamURL string `json:"upstream_url"`
	ListenAddr  string `json:"listen_addr"`
	APIKey      string `json:"api_key,omitempty"`
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

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
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
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
