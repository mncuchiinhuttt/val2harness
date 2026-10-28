package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/exporter"
	valSync "val2harness/internal/sync"
	"val2harness/internal/tui"
	"val2harness/internal/valclient"
	"val2harness/internal/version"
)

func runStart(args []string) {
	cfg := loadCLIConfig()
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
	cfg := loadCLIConfig()
	if err := daemon.StopDaemon(cfg.ListenAddr); err != nil {
		fmt.Fprintf(os.Stderr, "[Error] %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[OK] Server stopped successfully.")
}

func runStatus(args []string) {
	cfg := loadCLIConfig()
	running, pid := daemon.GetStatus(cfg.ListenAddr)
	if running {
		fmt.Printf("[RUNNING] Server active on http://%s (PID: %d)\n", cfg.ListenAddr, pid)
	} else if pid > 0 {
		fmt.Printf("[NEEDS ATTENTION] Process %d is alive; check its address, identity, or health before restarting.\n", pid)
	} else {
		fmt.Printf("[STOPPED] Server is not running on %s\n", cfg.ListenAddr)
	}
}

func runOpen() {
	cfg := loadCLIConfig()
	dashboard, err := daemon.DashboardURL(cfg)
	if err != nil {
		var transport *url.Error
		if errors.As(err, &transport) {
			if _, startErr := daemon.StartDaemon(cfg.ListenAddr); startErr != nil {
				fmt.Fprintln(os.Stderr, startErr)
				return
			}
			dashboard, err = daemon.DashboardURL(cfg)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := tui.OpenBrowser(dashboard); err != nil {
		fmt.Fprintln(os.Stderr, "Could not open the dashboard:", err)
	}
}

func runChat(args []string) {
	cfg := loadCLIConfig()
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
	cfg := loadCLIConfig()
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
	cfg := loadCLIConfig()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	proxyBaseURL := config.LocalBaseURL(cfg.ListenAddr)
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
	cfg := loadCLIConfig()
	fmt.Print(valSync.GenerateEnvSnippet(cfg.ListenAddr, cfg.APIKey))
}

func runToken(args []string) {
	cfg := loadCLIConfig()
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
	cfg := loadCLIConfig()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	targetDir := "exports"
	if len(args) > 0 {
		targetDir = args[0]
	} else {
		var err error
		targetDir, err = exporter.NewRunDirectory(targetDir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not create a private export folder:", err)
			os.Exit(1)
		}
	}
	count, outDir, err := exporter.ExportAllChats(ctx, client, targetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Export incomplete: %d chats fully saved in %s; any partial files were retained. Use a fresh directory to retry.\n%v\n", count, outDir, err)
		os.Exit(1)
	}
	fmt.Printf("Export completed: %d chats written to %s/\n", count, outDir)
}

// Stop before any dependent side effect when private settings cannot be loaded.
func loadCLIConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not load local settings:", err)
		os.Exit(1)
	}
	return cfg
}

func runUpdate(args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if len(args) > 0 && (args[0] == "--check" || args[0] == "-c") {
		info, err := version.CheckUpdate(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update check failed: %v\n", err)
			os.Exit(1)
		}
		if info.HasUpdate {
			fmt.Printf("Update available: %s -> %s\nRun 'val2harness update' to install.\n", info.CurrentVersion, info.LatestVersion)
		} else {
			fmt.Printf("val2harness is up to date (%s).\n", info.CurrentVersion)
		}
		return
	}

	fmt.Printf("Checking for updates (current: %s)...\n", version.CurrentVersion)
	v, err := version.SelfUpdate(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
		os.Exit(1)
	}
	if v == version.CurrentVersion {
		fmt.Printf("Already running the latest version (%s).\n", v)
	} else {
		fmt.Printf("[OK] Successfully updated to %s!\n", v)
	}
}
