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
		"1. Start Server (Background Daemon)",
		"2. Stop Server",
		"3. Check Server Status & Health",
		"4. Open Web Dashboard in Browser",
		"5. Quick Chat in Terminal (Test Opus 4.7/5)",
		"6. Sync Models to ALL Harnesses (OMP, Pi, OpenCode, Continue)",
		"7. Show Shell Environment Variables (Claude Code, Cursor, zcode)",
		"8. View Available Models",
		"9. Change Listen Port",
		"10. Update RMIT Val Token",
		"11. Export Chat History to Markdown",
		"0. Exit",
	}

	for {
		cfg, _ := config.Load()
		client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
		running, pid := daemon.GetStatus(cfg.ListenAddr)

		statusStr := "[STOPPED]"
		if running {
			statusStr = fmt.Sprintf("[RUNNING (PID %d)]", pid)
		}

		header := fmt.Sprintf(
			"+===========================================================+\r\n"+
				"| val2harness - RMIT Val AI Bridge                           |\r\n"+
				"| Status: %-18s  Port: %-22s |\r\n"+
				"+===========================================================+\r\n\r\n",
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
			} else {
				fmt.Printf("[Offline] Server is not running on %s\n", cfg.ListenAddr)
			}
		case 3: // Open WebUI
			url := fmt.Sprintf("http://%s/", cfg.ListenAddr)
			fmt.Printf("Opening %s in browser...\n", url)
			openBrowser(url)
		case 4: // Terminal Chat
			if cfg.Token == "" {
				fmt.Println("[Error] Token not set. Please set token first.")
			} else {
				RunTerminalChat(cfg, client, "anthropic-claude-opus-4-7")
			}
		case 5: // Sync All
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			results := valSync.SyncAll(ctx, client, "http://"+cfg.ListenAddr, cfg.APIKey)
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
			fmt.Println(valSync.GenerateEnvSnippet(cfg.ListenAddr))
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
			cnt, dir, err := exporter.ExportAllChats(ctx, client, "exports")
			cancel()
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
