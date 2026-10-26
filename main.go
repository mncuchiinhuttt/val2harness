package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/exporter"
	"val2harness/internal/omp"
	"val2harness/internal/proxy"
	"val2harness/internal/tui"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

func main() {
	if len(os.Args) < 2 {
		tui.RunTUI()
		return
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "tui":
		tui.RunTUI()
	case "start":
		runStart(args)
	case "stop":
		runStop(args)
	case "status":
		runStatus(args)
	case "serve":
		runServe(args)
	case "chat":
		runChat(args)
	case "open":
		runOpen()
	case "models":
		runModels(args)
	case "sync-omp":
		runSyncOMP(args)
	case "token":
		runToken(args)
	case "export":
		runExport(args)
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'val2harness help' for usage.\n", cmd)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`val2harness - RMIT Val AI Bridge for OMP and Agent Harnesses

Usage:
  val2harness [command] [options]

Commands:
  (none)      Launch interactive Terminal UI (default)
  start       Start proxy server in background (daemon)
  stop        Stop running background proxy server
  status      Check background server status and health
  serve       Run proxy server in foreground
  chat        Quick interactive streaming chat in terminal
  open        Open web dashboard in default browser
  models      List all models available on your RMIT Val account
  sync-omp    Sync models directly into ~/.omp/models.yml
  token       View or update the RMIT Val JWT token
  export      Export chat conversations to Markdown and JSON
  help        Show this help message

Options:
  -addr string       Listen address (default "127.0.0.1:8765")
  -upstream string   RMIT Val upstream URL (default "https://val.rmit.edu.au")`)
}

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
	_, _ = daemon.StartDaemon(cfg.ListenAddr) // Ensure running
	tui.RunTUI()                          // Re-enter or open
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

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "Listen address")
	upstream := fs.String("upstream", "", "Upstream URL")
	_ = fs.Parse(args)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
	}
	if *addr != "" {
		cfg.ListenAddr = *addr
	}
	if *upstream != "" {
		cfg.UpstreamURL = *upstream
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	srv := proxy.NewServer(cfg, client)

	web.RegisterWebRoutes(srv.GetMux(), cfg, client, func(newToken string) {
		srv.UpdateToken(newToken)
	})

	fmt.Printf("val2harness proxy listening on http://%s\n", cfg.ListenAddr)
	fmt.Printf("Web Dashboard:  http://%s/\n", cfg.ListenAddr)
	fmt.Printf("OpenAI BaseUrl: http://%s/v1\n", cfg.ListenAddr)
	fmt.Printf("Upstream Val:   %s\n", cfg.UpstreamURL)
	if cfg.Token == "" {
		fmt.Println("[WARNING] No token configured. Visit http://" + cfg.ListenAddr + "/ to set your token.")
	} else {
		fmt.Println("[OK] Token is loaded.")
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-stop
	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
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

func runSyncOMP(args []string) {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	proxyBaseURL := "http://" + cfg.ListenAddr
	count, path, err := omp.SyncModels(ctx, client, proxyBaseURL, cfg.APIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully synced %d models to %s\n", count, path)
	fmt.Printf("Provider 'rmit-val' is now active pointing to %s/v1\n", proxyBaseURL)
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
