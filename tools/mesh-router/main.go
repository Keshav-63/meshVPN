package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
)

// --- CACHE IMPLEMENTATION ---
// RWMutex allows thousands of concurrent reads without blocking, only locking on writes.
type CacheEntry struct {
	TailscaleIP string
	ExpiresAt   time.Time
}

type RouterCache struct {
	mu    sync.RWMutex
	items map[string]CacheEntry
	ttl   time.Duration
}

func NewRouterCache(ttl time.Duration) *RouterCache {
	return &RouterCache{
		items: make(map[string]CacheEntry),
		ttl:   ttl,
	}
}

func (c *RouterCache) Get(subdomain string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.items[subdomain]
	if !found || time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.TailscaleIP, true
}

func (c *RouterCache) Set(subdomain, ip string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[subdomain] = CacheEntry{
		TailscaleIP: ip,
		ExpiresAt:   time.Now().Add(c.ttl),
	}
}

// --- GLOBALS ---
var (
	db    *sql.DB
	cache *RouterCache
)

func initDB() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/meshvpn?sslmode=disable"
	}
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	// Connection pool tuning for low latency
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
}

func getWorkerIP(subdomain string) (string, error) {
	// 1. Check ultra-fast L1 In-Memory Cache
	if ip, found := cache.Get(subdomain); found {
		return ip, nil
	}

	// 2. Cache Miss -> Query L2 Postgres
	var tailscaleIP sql.NullString
	var ownerWorkerID sql.NullString
	query := `
		SELECT w.tailscale_ip, d.owner_worker_id
		FROM deployments d 
		LEFT JOIN workers w ON d.owner_worker_id = w.worker_id 
		WHERE d.subdomain = $1 AND d.status = 'running'
	`
	err := db.QueryRow(query, subdomain).Scan(&tailscaleIP, &ownerWorkerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("deployment offline or not found")
		}
		return "", err
	}

	// If the IP is NULL (e.g., control-plane-local doesn't record it),
	// route to 127.0.0.1 where the control plane Traefik runs.
	ip := "127.0.0.1"
	if tailscaleIP.Valid && tailscaleIP.String != "" {
		ip = tailscaleIP.String
	} else if !ownerWorkerID.Valid || ownerWorkerID.String != "control-plane-local" {
		log.Printf("⚠️ Warning: worker_id '%s' has NO tailscale_ip for subdomain '%s'. Falling back to 127.0.0.1", ownerWorkerID.String, subdomain)
	}

	// 3. Populate Cache
	cache.Set(subdomain, ip)
	return ip, nil
}

func meshProxyHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received request from Cloudflare: %s %s", r.Method, r.Host)

	host := r.Host
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		log.Printf("❌ Invalid host format: %s", host)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	subdomain := parts[0]

	// Look up routing IP
	targetIP, err := getWorkerIP(subdomain)
	if err != nil {
		log.Printf("❌ 404: Deployment Not Found for subdomain '%s': %v", subdomain, err)
		http.Error(w, "404 - Deployment Not Found", http.StatusNotFound)
		return
	}

	log.Printf("✅ Routing subdomain '%s' to worker at %s:80", subdomain, targetIP)

	// Proxy to Worker over Tailscale
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:80", targetIP))
	if err != nil {
		log.Printf("❌ Failed to parse target URL %s: %v", targetIP, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	log.Printf("🔄 Forwarding proxy target configured as: %s", targetURL.String())

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Error handler to catch 502s from the reverse proxy itself
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		log.Printf("🚨 ПРОXY ERROR [Gateway Timeout] brdiging to %s for %s", targetIP, req.Host)
		log.Printf("🚨 Error Details: %v", err)
		rw.WriteHeader(http.StatusBadGateway)
		rw.Write([]byte(fmt.Sprintf("MeshVPN Edge Router: Failed to reach worker at %s\nError: %v", targetIP, err)))
	}

	// Transport debug
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: 10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext,
	}

	// CRITICAL: Rewrite Director to preserve original host for Traefik
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = host
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Mesh-Routed", "true")
		log.Printf("✈️ Dispatching outgoing request to: %s%s", targetURL.String(), req.URL.Path)
	}

	proxy.ServeHTTP(w, r)
}

func main() {
	initDB()
	defer db.Close()

	// Cache TTL set to 15 seconds. High enough to absorb traffic spikes,
	// low enough to react quickly to failovers.
	cache = NewRouterCache(15 * time.Second)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082" // Control plane runs on 8080, traffic-forwarder on 8081, this runs on 8082
	}

	http.HandleFunc("/", meshProxyHandler)

	log.Printf("🚀 Mesh Router running on 0.0.0.0:%s (with RWMutex Cache)", port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, nil))
}
