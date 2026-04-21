package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Target: control-plane running on WSL localhost
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	// Verify target is reachable before starting
	if err := checkTargetHealth(target.String()); err != nil {
		log.Printf("⚠️  Warning: target not immediately available: %v (will retry on requests)", err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "Bad Gateway")
	}

	// Create HTTP mux to handle both health checks and proxy
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "OK")
	})

	// Proxy all other requests
	mux.Handle("/", proxy)

	// Listen on all interfaces so Docker can reach via host.docker.internal
	// For Docker Desktop on WSL, also try to bind to the gateway IP
	listenAddr := "0.0.0.0:8081"
	
	// First, check if we should use host.docker.internal gateway
	// Docker Desktop WSL gateway is typically 192.168.65.254
	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	log.Printf("Starting proxy server on %s", listenAddr)
	log.Printf("Forwarding to %s", target.String())

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
		os.Exit(0)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// checkTargetHealth verifies target is reachable
func checkTargetHealth(targetURL string) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return io.EOF
	}
	return nil
}
