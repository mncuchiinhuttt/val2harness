package main

import (
	"fmt"
	"os"

	"val2harness/internal/tui"
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
	case "sync", "sync-omp":
		runSync(args)
	case "env":
		runEnv()
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
  sync        Sync models to ALL harnesses (OMP, Pi, OpenCode, Continue)
  env         Print shell export variables (for Claude Code, Cursor, Aider, zcode)
  token       View or update the RMIT Val JWT token
  export      Export chat conversations to Markdown and JSON
  help        Show this help message

Options:
  -addr string       Listen address (default "127.0.0.1:8765")
  -upstream string   RMIT Val upstream URL (default "https://val.rmit.edu.au")`)
}
