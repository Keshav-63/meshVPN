package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// Target: control-plane running on WSL localhost
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Custom error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "Bad Gateway")
	}

	// Listen on all interfaces so Docker can reach via host.docker.internal
	listenAddr := "0.0.0.0:8081"

	log.Printf("Starting proxy server on %s", listenAddr)
	log.Printf("Forwarding to %s", target.String())

	if err := http.ListenAndServe(listenAddr, proxy); err != nil {
		log.Fatal(err)
	}
}
