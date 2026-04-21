# Bridge Proxy Issues and Fixes

## **Main Issues Identified**

### 1. ❌ Missing `/health` Endpoint (CRITICAL)
**Problem:** The `deploy.sh` script checks for a `/health` endpoint that doesn't exist:
```bash
docker exec k3d-meshvpn-server-0 sh -c "wget -qO- --timeout=2 http://host.docker.internal:8081/health"
```
But `bridge-proxy.go` only handles reverse proxy requests - no health check endpoint.

**Impact:** Health check fails (404) → deployment script exits and kills the proxy

**Fix:** Added `/health` endpoint to `bridge-proxy.go`:
```go
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    io.WriteString(w, "OK")
})
```

---

### 2. ❌ No Graceful Shutdown Handling
**Problem:** When the proxy receives SIGTERM or SIGINT, it abruptly terminates instead of gracefully shutting down.

**Fix:** Added signal handling:
```go
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
```

---

### 3. ⚠️ No Control-Plane Availability Check
**Problem:** If `localhost:8080` (control-plane) isn't running, the proxy starts but fails silently when trying to forward traffic.

**Fix in deploy.sh:** Added pre-flight check:
```bash
if ! nc -zv localhost 8080 2>/dev/null; then
    echo "⚠️  Warning: Control-plane not accessible on localhost:8080"
fi
```

---

### 4. ⚠️ Poor Error Reporting
**Problem:** When proxy start fails, script just says "check bridge-proxy.log" but doesn't show what's in it.

**Fix:** Enhanced error output:
```bash
echo "   Recent logs:"
tail -10 bridge-proxy.log
echo "   Manual start: ./bridge-proxy.exe &"
echo "   Check logs: tail -f bridge-proxy.log"
```

---

## **What Was Changed**

### Files Modified:
1. **bridge-proxy.go** - Added:
   - `/health` endpoint for health checks
   - Signal handling (SIGINT, SIGTERM)
   - Target availability check
   - Proper server shutdown logic
   - Better error messages

2. **deploy.sh** - Enhanced:
   - Control-plane connectivity check before starting proxy
   - Better error messages with log tail
   - Improved diagnostics

---

## **How to Test**

1. **Rebuild the proxy:**
   ```bash
   cd tools/traffic-forwarder
   go build -o bridge-proxy.exe bridge-proxy.go
   ```

2. **Test locally:**
   ```bash
   ./bridge-proxy.exe &
   
   # In another terminal:
   curl http://localhost:8081/health  # Should return OK
   curl http://localhost:8081/        # Should reverse-proxy to localhost:8080
   
   # Kill with Ctrl+C - should gracefully shutdown
   ```

3. **Run deployment:**
   ```bash
   ./deploy.sh
   ```

---

## **Next Steps**

- [ ] Rebuild bridge-proxy.exe with updated Go code
- [ ] Test `/health` endpoint responds correctly
- [ ] Verify graceful shutdown works
- [ ] Re-run `./deploy.sh` to validate proxy starts successfully
