package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/exporter"
	valSync "val2harness/internal/sync"
	"val2harness/internal/tui"
	"val2harness/internal/valclient"
)

func runStart(args []string) {
	cfg, _ := config.Load()
	pid, err := daemon.StartDaemon(cfg.ListenAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Server started in background with PID %d\n", pid)
	fmt.Printf("Dashboard:  http://%s/\n", cfg.ListenAddr)
	fmt.Printf("OpenAI API: http://%s/v1\n", cfg.ListenAddr)
}

func runStop(args []string) {
	cfg, _ := config.Load()
	if err := daemon.StopDaemon(cfg.ListenAddr); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[OK] Server stopped successfully.")
}

func runStatus(args []string) {
	cfg, _ := config.Load()
	running, pid := daemon.GetStatus(cfg.ListenAddr)
	if running {
		fmt.Printf("[RUNNING] Server active on http://%s (PID: %d)\n", cfg.ListenAddr, pid)
	} else {
		fmt.Printf("[STOPPED] Server is not running on %s\n", cfg.ListenAddr)
	}
}

func runOpen() {
	cfg, _ := config.Load()
	url := fmt.Sprintf("http://%s/", cfg.ListenAddr)
	fmt.Printf("Opening %s in browser...\n", url)
	_, _ = daemon.StartDaemon(cfg.ListenAddr)
	tui.RunTUI()
}

func runChat(args []string) {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not set. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}
	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	model := "anthropic-claude-opus-4-7"
	if len(args) > 0 {
		model = args[0]
	}
	tui.RunTerminalChat(cfg, client, model)
}

func runModels(args []string) {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	models, err := client.ListModels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch models: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d models on RMIT Val:\n\n", len(models))
	for _, m := range models {
		fmt.Printf("  - %-45s (%s)\n", m.ID, m.Name)
	}
}

func runSync(args []string) {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	proxyBaseURL := "http://" + cfg.ListenAddr
	results := valSync.SyncAll(ctx, client, proxyBaseURL, cfg.APIKey)
	fmt.Println("Harness Sync Results:")
	for _, res := range results {
		if res.Error != nil {
			fmt.Printf("  [FAILED] %-35s -> %v\n", res.Harness, res.Error)
		} else {
			fmt.Printf("  [OK]     %-35s -> Synced %d models (%s)\n", res.Harness, res.Count, res.Path)
		}
	}
}

func runEnv() {
	cfg, _ := config.Load()
	fmt.Print(valSync.GenerateEnvSnippet(cfg.ListenAddr))
}

func runToken(args []string) {
	cfg, _ := config.Load()
	if len(args) == 0 {
		if cfg.Token == "" {
			fmt.Println("No token is currently configured.")
		} else {
			preview := cfg.Token
			if len(preview) > 30 {
				preview = preview[:25] + "..."
			}
			fmt.Printf("Current token configured: %s\n", preview)
		}
		return
	}

	newToken := strings.TrimSpace(args[0])
	cfg.Token = newToken
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save token: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Token saved successfully to configuration.")
}

func runExport(args []string) {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	count, outDir, err := exporter.ExportAllChats(ctx, client, "exports")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Export completed: %d chats written to %s/\n", count, outDir)
}
