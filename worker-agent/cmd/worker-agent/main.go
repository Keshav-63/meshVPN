package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"worker-agent/internal/agent"
	"worker-agent/internal/config"
)

func main() {
	configPath := flag.String("config", "agent.yaml", "path to configuration file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Auto-detect Tailscale IP if not provided
	if cfg.Worker.TailscaleIP == "" {
		cfg.Worker.TailscaleIP = detectTailscaleIP()
		if cfg.Worker.TailscaleIP == "" {
			log.Fatal("failed to detect Tailscale IP. Please set tailscale_ip in config or install Tailscale")
		}
		log.Printf("Auto-detected Tailscale IP: %s", cfg.Worker.TailscaleIP)
	}

	// Create and start agent
	workerAgent := agent.New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go workerAgent.Start(ctx)

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Shutting down worker agent...")
	cancel()
	time.Sleep(2 * time.Second)
}

func detectTailscaleIP() string {
	cmd := exec.Command("tailscale", "ip", "-4")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
