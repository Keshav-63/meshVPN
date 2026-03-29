package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TraefikAccessLog represents a Traefik access log entry
type TraefikAccessLog struct {
	ClientHost       string  `json:"ClientHost"`
	RequestHost      string  `json:"RequestHost"`
	RequestMethod    string  `json:"RequestMethod"`
	RequestPath      string  `json:"RequestPath"`
	OriginStatus     int     `json:"OriginStatus"`
	Duration         int64   `json:"Duration"` // Microseconds
	OriginContentSize int64  `json:"OriginContentSize"`
	RequestContentSize int64 `json:"RequestContentSize"`
	Time             string  `json:"time"`
}

// TelemetryPayload for control-plane endpoint
type TelemetryPayload struct {
	DeploymentID  string  `json:"deployment_id"`
	StatusCode    int     `json:"status_code"`
	LatencyMs     float64 `json:"latency_ms"`
	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	Path          string  `json:"path"`
	Timestamp     string  `json:"timestamp"`
}

func main() {
	controlPlaneURL := os.Getenv("CONTROL_PLANE_URL")
	if controlPlaneURL == "" {
		controlPlaneURL = "http://localhost:8080"
	}

	traefikNamespace := os.Getenv("TRAEFIK_NAMESPACE")
	if traefikNamespace == "" {
		traefikNamespace = "kube-system"
	}

	traefikPod := os.Getenv("TRAEFIK_POD")
	if traefikPod == "" {
		// Auto-detect Traefik pod
		cmd := exec.Command("kubectl", "get", "pods", "-n", traefikNamespace,
			"-l", "app.kubernetes.io/name=traefik", "-o", "jsonpath={.items[0].metadata.name}")
		output, err := cmd.Output()
		if err != nil {
			log.Fatalf("Failed to find Traefik pod: %v", err)
		}
		traefikPod = strings.TrimSpace(string(output))
	}

	if traefikPod == "" {
		log.Fatal("Could not find Traefik pod. Set TRAEFIK_POD environment variable.")
	}

	log.Printf("Starting traffic forwarder...")
	log.Printf("Control Plane: %s", controlPlaneURL)
	log.Printf("Traefik Pod: %s (namespace: %s)", traefikPod, traefikNamespace)

	// Start tailing Traefik access logs
	cmd := exec.Command("kubectl", "logs", "-n", traefikNamespace, "-f", traefikPod, "-c", "traefik")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start kubectl logs: %v", err)
	}

	log.Println("Tailing Traefik access logs...")

	// Read logs line by line
	buf := make([]byte, 0, 4096)
	for {
		tmp := make([]byte, 1024)
		n, err := stdout.Read(tmp)
		if err != nil {
			log.Printf("Read error: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		buf = append(buf, tmp[:n]...)

		// Process complete lines
		for {
			idx := bytes.IndexByte(buf, '\n')
			if idx == -1 {
				break
			}

			line := buf[:idx]
			buf = buf[idx+1:]

			// Process log line
			processLogLine(string(line), controlPlaneURL)
		}
	}
}

func processLogLine(line string, controlPlaneURL string) {
	// Parse JSON log
	var accessLog TraefikAccessLog
	if err := json.Unmarshal([]byte(line), &accessLog); err != nil {
		// Skip non-JSON lines
		return
	}

	// Extract deployment ID from RequestHost
	// Example: myapp.keshavstack.tech -> myapp
	subdomain := extractSubdomain(accessLog.RequestHost)
	if subdomain == "" {
		return // Not a user deployment
	}

	// Convert to telemetry payload
	payload := TelemetryPayload{
		DeploymentID:  subdomain, // We'll need to map subdomain -> deployment_id
		StatusCode:    accessLog.OriginStatus,
		LatencyMs:     float64(accessLog.Duration) / 1000.0, // Convert microseconds to milliseconds
		BytesSent:     accessLog.OriginContentSize,
		BytesReceived: accessLog.RequestContentSize,
		Path:          accessLog.RequestPath,
		Timestamp:     accessLog.Time,
	}

	// Send to telemetry endpoint
	if err := sendTelemetry(controlPlaneURL, payload); err != nil {
		log.Printf("Failed to send telemetry: %v", err)
	}
}

func extractSubdomain(host string) string {
	// Extract subdomain from host like: myapp.keshavstack.tech -> myapp
	parts := strings.Split(host, ".")
	if len(parts) < 3 {
		return ""
	}

	subdomain := parts[0]

	// Skip non-deployment hosts
	if subdomain == "www" || subdomain == "api" || subdomain == "control-plane" {
		return ""
	}

	return subdomain
}

func sendTelemetry(baseURL string, payload TelemetryPayload) error {
	endpoint := baseURL + "/api/telemetry/deployment-request"

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telemetry endpoint returned status %d", resp.StatusCode)
	}

	return nil
}
