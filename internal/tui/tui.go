package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/daemon"
	"val2harness/internal/exporter"
	valSync "val2harness/internal/sync"
	"val2harness/internal/valclient"
)

func RunTUI() {
	reader := bufio.NewReader(os.Stdin)
	selectedIdx := 0

	items := []string{
		"Start Server (Background Daemon)",
		"Stop Server",
		"Check Server Status & Health",
		"Open Web Dashboard in Browser",
		"Quick Chat in Terminal (Test Opus 4.7/5)",
		"Sync Models to ALL Harnesses (OMP, Pi, OpenCode, Continue)",
		"Show Shell Environment Variables (Claude Code, Cursor, zcode)",
		"View Available Models",
		"Change Listen Port",
		"Update RMIT Val Token",
		"Export Chat History to Markdown",
		"Exit",
	}

	for {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not load local settings:", err)
			return
		}
		client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
		running, pid := daemon.GetStatus(cfg.ListenAddr)

		statusStr := "\x1b[33m● Stopped\x1b[0m"
		if running {
			statusStr = fmt.Sprintf("\x1b[32m● Running (PID %d)\x1b[0m", pid)
		} else if pid > 0 {
			statusStr = fmt.Sprintf("\x1b[33m● Needs attention (PID %d)\x1b[0m", pid)
		}

		header := fmt.Sprintf(
			"\r\n\x1b[1mval2harness\x1b[0m \x1b[90m·\x1b[0m RMIT Val AI Bridge\r\n"+
				"Status: %s \x1b[90m·\x1b[0m Port: \x1b[1m%s\x1b[0m\r\n"+
				"\x1b[90m────────────────────────────────────────────────────────────\x1b[0m\r\n\r\n",
			statusStr, cfg.ListenAddr,
		)

		idx := SelectMenu(header, items, selectedIdx)
		if idx == -1 || idx == len(items)-1 {
			fmt.Println("\r\nGoodbye.")
			return
		}
		selectedIdx = idx

		// Clear screen for action output
		fmt.Print("\x1b[H\x1b[2J")

		switch idx {
		case 0: // Start
			fmt.Printf("Starting daemon on %s...\n", cfg.ListenAddr)
			newPID, err := daemon.StartDaemon(cfg.ListenAddr)
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Printf("[OK] Server running in background with PID %d\n", newPID)
				fmt.Printf("Dashboard: http://%s/\n", cfg.ListenAddr)
			}
		case 1: // Stop
			fmt.Println("Stopping daemon...")
			if err := daemon.StopDaemon(cfg.ListenAddr); err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Println("[OK] Server stopped.")
			}
		case 2: // Status
			r, p := daemon.GetStatus(cfg.ListenAddr)
			if r {
				fmt.Printf("[OK] Server is healthy and listening on http://%s (PID: %d)\n", cfg.ListenAddr, p)
			} else if p > 0 {
				fmt.Printf("[Needs attention] Process %d is alive; check its address, identity, or health before restarting.\n", p)
			} else {
				fmt.Printf("[Offline] Server is not running on %s\n", cfg.ListenAddr)
			}
		case 3: // Open WebUI
			url, err := daemon.DashboardURL(cfg)
			if err != nil {
				fmt.Println("Start the gateway first, then open its dashboard:", err)
			} else if err := OpenBrowser(url); err != nil {
				fmt.Println("Could not open dashboard:", err)
			}
		case 4: // Terminal Chat
			if cfg.Token == "" {
				fmt.Println("[Error] Token not set. Please set token first.")
			} else {
				RunTerminalChat(cfg, client, "anthropic-claude-opus-4-7")
			}
		case 5: // Sync All
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			results := valSync.SyncAll(ctx, client, config.LocalBaseURL(cfg.ListenAddr), cfg.APIKey)
			cancel()
			fmt.Println("Harness Sync Results:")
			for _, res := range results {
				if res.Error != nil {
					fmt.Printf("  [FAILED] %-35s -> %v\n", res.Harness, res.Error)
				} else {
					fmt.Printf("  [OK]     %-35s -> Synced %d models (%s)\n", res.Harness, res.Count, res.Path)
				}
			}
		case 6: // Env Snippet
			fmt.Println(valSync.GenerateEnvSnippet(cfg.ListenAddr, cfg.APIKey))
		case 7: // Models
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			models, err := client.ListModels(ctx)
			cancel()
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Printf("Found %d models on RMIT Val:\n\n", len(models))
				for _, m := range models {
					fmt.Printf("  - %-40s (%s)\n", m.ID, m.Name)
				}
			}
		case 8: // Change Port
			fmt.Printf("Current address is '%s'. Enter new address (e.g. 127.0.0.1:9292): ", cfg.ListenAddr)
			newAddr, _ := reader.ReadString('\n')
			newAddr = strings.TrimSpace(newAddr)
			if newAddr != "" {
				cfg.ListenAddr = newAddr
				_ = cfg.Save()
				fmt.Printf("[OK] Listen address updated to %s\n", newAddr)
			}
		case 9: // Update Token
			fmt.Print("Paste new RMIT Val JWT token: ")
			newToken, _ := reader.ReadString('\n')
			newToken = strings.TrimSpace(newToken)
			if newToken != "" {
				cfg.Token = newToken
				_ = cfg.Save()
				fmt.Println("[OK] Token saved successfully.")
			}
		case 10: // Export
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			dir, err := exporter.NewRunDirectory("exports")
			if err != nil {
				cancel()
				fmt.Println("[Error] Could not create a private export folder:", err)
				break
			}
			cnt, dir, err := exporter.ExportAllChats(ctx, client, dir)
			cancel()
			if err != nil {
				fmt.Printf("[Incomplete] %d chats fully saved in %s; partial files were retained. Use a fresh directory to retry.\n%v\n", cnt, dir, err)
			} else {
				fmt.Printf("[OK] Exported %d chats to %s/\n", cnt, dir)
			}
		}

		if idx != 4 { // If not chat (chat handles its own pause)
			fmt.Print("\nPress Enter to return to menu...")
			if _, err := reader.ReadString('\n'); err != nil {
				return
			}
		}
	}
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
