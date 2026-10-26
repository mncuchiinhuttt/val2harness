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

	for {
		cfg, _ := config.Load()
		client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
		running, pid := daemon.GetStatus(cfg.ListenAddr)

		statusStr := "[STOPPED]"
		if running {
			statusStr = fmt.Sprintf("[RUNNING (PID %d)]", pid)
		}

		fmt.Println("\n+===========================================================+")
		fmt.Printf("| val2harness - RMIT Val AI Bridge                           |\n")
		fmt.Printf("| Status: %-18s  Port: %-22s |\n", statusStr, cfg.ListenAddr)
		fmt.Println("+===========================================================+")
		fmt.Println("  1. Start Server (Background Daemon)")
		fmt.Println("  2. Stop Server")
		fmt.Println("  3. Check Server Status & Health")
		fmt.Println("  4. Open Web Dashboard in Browser")
		fmt.Println("  5. Quick Chat in Terminal (Test Opus 4.7/5)")
		fmt.Println("  6. Sync Models to ALL Harnesses (OMP, Pi, OpenCode, Continue)")
		fmt.Println("  7. Show Shell Environment Variables (Claude Code, Cursor, zcode)")
		fmt.Println("  8. View Available Models")
		fmt.Println("  9. Change Listen Port")
		fmt.Println(" 10. Update RMIT Val Token")
		fmt.Println(" 11. Export Chat History to Markdown")
		fmt.Println("  0. Exit")
		fmt.Print("\nSelect an option [0-11]: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Printf("Starting daemon on %s...\n", cfg.ListenAddr)
			newPID, err := daemon.StartDaemon(cfg.ListenAddr)
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Printf("[OK] Server running in background with PID %d\n", newPID)
				fmt.Printf("Dashboard: http://%s/\n", cfg.ListenAddr)
			}
		case "2":
			fmt.Println("Stopping daemon...")
			if err := daemon.StopDaemon(cfg.ListenAddr); err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Println("[OK] Server stopped.")
			}
		case "3":
			r, p := daemon.GetStatus(cfg.ListenAddr)
			if r {
				fmt.Printf("[OK] Server is healthy and listening on http://%s (PID: %d)\n", cfg.ListenAddr, p)
			} else {
				fmt.Printf("[Offline] Server is not running on %s\n", cfg.ListenAddr)
			}
		case "4":
			url := fmt.Sprintf("http://%s/", cfg.ListenAddr)
			fmt.Printf("Opening %s in browser...\n", url)
			openBrowser(url)
		case "5":
			if cfg.Token == "" {
				fmt.Println("[Error] Token not set. Please set token first.")
			} else {
				RunTerminalChat(cfg, client, "anthropic-claude-opus-4-7")
			}
		case "6":
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			results := valSync.SyncAll(ctx, client, "http://"+cfg.ListenAddr, cfg.APIKey)
			cancel()
			fmt.Println("\nSync Results:")
			for _, res := range results {
				if res.Error != nil {
					fmt.Printf("  [FAILED] %-35s -> %v\n", res.Harness, res.Error)
				} else {
					fmt.Printf("  [OK]     %-35s -> Synced %d models (%s)\n", res.Harness, res.Count, res.Path)
				}
			}
		case "7":
			fmt.Println("\n" + valSync.GenerateEnvSnippet(cfg.ListenAddr))
		case "8":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			models, err := client.ListModels(ctx)
			cancel()
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Printf("\nFound %d models:\n", len(models))
				for _, m := range models {
					fmt.Printf("  - %-40s (%s)\n", m.ID, m.Name)
				}
			}
		case "9":
			fmt.Printf("Current address is '%s'. Enter new address (e.g. 127.0.0.1:9292): ", cfg.ListenAddr)
			newAddr, _ := reader.ReadString('\n')
			newAddr = strings.TrimSpace(newAddr)
			if newAddr != "" {
				cfg.ListenAddr = newAddr
				_ = cfg.Save()
				fmt.Printf("[OK] Listen address updated to %s\n", newAddr)
			}
		case "10":
			fmt.Print("Paste new RMIT Val JWT token: ")
			newToken, _ := reader.ReadString('\n')
			newToken = strings.TrimSpace(newToken)
			if newToken != "" {
				cfg.Token = newToken
				_ = cfg.Save()
				fmt.Println("[OK] Token saved successfully.")
			}
		case "11":
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			cnt, dir, err := exporter.ExportAllChats(ctx, client, "exports")
			cancel()
			if err != nil {
				fmt.Printf("[Error] %v\n", err)
			} else {
				fmt.Printf("[OK] Exported %d chats to %s/\n", cnt, dir)
			}
		case "0", "q", "exit":
			fmt.Println("Goodbye.")
			return
		default:
			fmt.Println("Invalid option, please choose between 0 and 11.")
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
