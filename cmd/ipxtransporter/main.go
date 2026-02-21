// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Main entry point

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mlapointe/ipxtransporter/internal/api"
	"github.com/mlapointe/ipxtransporter/internal/config"
	"github.com/mlapointe/ipxtransporter/internal/relay"
	"github.com/mlapointe/ipxtransporter/internal/tui"
	"github.com/spf13/pflag"
)

func main() {
	configPath := pflag.String("config", "/etc/ipxtransporter.json", "Path to config file")
	iface := pflag.String("interface", "", "Network interface to capture from")
	listenAddr := pflag.String("listen", "", "TLS listen address")
	disableSSL := pflag.Bool("disable-ssl", false, "Disable TLS (debug only)")
	tuiMode := pflag.Bool("tui", true, "Enable TUI mode")
	demoMode := pflag.Bool("demo", false, "Enable demo mode with fake traffic")
	pflag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: failed to load config from %s: %v. Using defaults.", *configPath, err)
	}

	// Override config with flags if provided
	if *iface != "" {
		cfg.Interface = *iface
	}
	if *listenAddr != "" {
		cfg.ListenAddr = *listenAddr
	}
	if *disableSSL {
		cfg.DisableSSL = true
	}

	srv, err := relay.NewServer(cfg, *configPath)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if *demoMode {
		srv.SetDemoMode(true)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	if cfg.EnableHTTP {
		apiSrv := api.NewAPI(srv, cfg)
		go func() {
			if err := apiSrv.ListenAndServe(cfg.HTTPListenAddr); err != nil {
				log.Printf("HTTP API error: %v", err)
			}
		}()
	}

	if *tuiMode {
		tuiApp := tui.NewTUIWithDemo(srv.CollectStats, cfg, *configPath, srv.UpdateDemoProps, srv.DisconnectPeer, srv.BanPeer)
		if err := tuiApp.Run(ctx); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
	} else {
		log.Println("Running in daemon mode. Press Ctrl+C to exit.")
		<-ctx.Done()
	}
}
