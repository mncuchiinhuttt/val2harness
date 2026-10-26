package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/valclient"
)

func runQuota() {
	cfg, _ := config.Load()
	if cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: Token not configured. Run 'val2harness token <jwt>' first.")
		os.Exit(1)
	}

	client := valclient.NewClient(cfg.UpstreamURL, cfg.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Println("Probing RMIT Val daily quota status...")
	q, err := client.CheckQuota(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Quota check failed: %v\n", err)
		os.Exit(1)
	}

	statusStr := "[OK] Active"
	if q.Status == "exceeded" {
		statusStr = "[EXCEEDED] Daily Limit Reached"
	}

	fmt.Printf("\nRMIT Val Daily Quota Status:\n")
	fmt.Printf("  Status:             %s\n", statusStr)
	fmt.Printf("  Message:            %s\n", q.Message)
	fmt.Printf("  Melbourne Time:     %s\n", q.MelbourneTime)
	fmt.Printf("  Next 4AM Reset:     %s (in %s)\n", q.ResetAtMelbourne, q.Countdown)
}
