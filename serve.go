package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/proxy"
	"val2harness/internal/valclient"
	"val2harness/internal/web"
)

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
