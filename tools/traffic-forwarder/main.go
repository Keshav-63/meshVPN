package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// TraefikAccessLog represents a Traefik access log entry
type TraefikAccessLog struct {
	ClientHost         string `json:"ClientHost"`
	RequestHost        string `json:"RequestHost"`
	RequestAddr        string `json:"RequestAddr"`
	RequestMethod      string `json:"RequestMethod"`
	RequestPath        string `json:"RequestPath"`
	OriginStatus       int    `json:"OriginStatus"`
	DownstreamStatus   int    `json:"DownstreamStatus"`
	Duration           int64  `json:"Duration"` // Microseconds
	OriginContentSize  int64  `json:"OriginContentSize"`
	RequestContentSize int64  `json:"RequestContentSize"`
	Time               string `json:"time"`
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

type BatchTelemetryPayload struct {
	Requests []TelemetryPayload `json:"requests"`
}

type TelemetrySender struct {
	batchEndpoint string
	client        *http.Client
	queue         chan TelemetryPayload
	batchSize     int
	flushInterval time.Duration

	dropped uint64
	sent    uint64
	failed  uint64
}

func NewTelemetrySender(baseURL string) *TelemetrySender {
	batchSize := envInt("TELEMETRY_BATCH_SIZE", 200)
	if batchSize < 1 {
		batchSize = 1
	}

	queueSize := envInt("TELEMETRY_QUEUE_SIZE", 20000)
	if queueSize < batchSize {
		queueSize = batchSize * 2
	}

	flushInterval := envDuration("TELEMETRY_FLUSH_INTERVAL", time.Second)
	if flushInterval < 100*time.Millisecond {
		flushInterval = 100 * time.Millisecond
	}

	httpTimeout := envDuration("TELEMETRY_HTTP_TIMEOUT", 3*time.Second)
	if httpTimeout < 500*time.Millisecond {
		httpTimeout = 500 * time.Millisecond
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	return &TelemetrySender{
		batchEndpoint: strings.TrimRight(baseURL, "/") + "/api/telemetry/deployment-request/batch",
		client: &http.Client{
			Timeout:   httpTimeout,
			Transport: transport,
		},
		queue:         make(chan TelemetryPayload, queueSize),
		batchSize:     batchSize,
		flushInterval: flushInterval,
	}
}

func (s *TelemetrySender) Enqueue(payload TelemetryPayload) {
	select {
	case s.queue <- payload:
	default:
		atomic.AddUint64(&s.dropped, 1)
	}
}

func (s *TelemetrySender) Run() {
	ticker := time.NewTicker(s.flushInterval)
	statsTicker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer statsTicker.Stop()

	batch := make([]TelemetryPayload, 0, s.batchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}

		if err := s.postBatch(batch); err != nil {
			// Single quick retry to reduce drop probability during transient network blips.
			time.Sleep(200 * time.Millisecond)
			if retryErr := s.postBatch(batch); retryErr != nil {
				atomic.AddUint64(&s.failed, uint64(len(batch)))
				log.Printf("Telemetry batch send failed size=%d err=%v", len(batch), retryErr)
				batch = batch[:0]
				return
			}
		}

		atomic.AddUint64(&s.sent, uint64(len(batch)))
		batch = batch[:0]
	}

	for {
		select {
		case payload := <-s.queue:
			batch = append(batch, payload)
			if len(batch) >= s.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-statsTicker.C:
			log.Printf(
				"Telemetry sender stats queue=%d sent=%d failed=%d dropped=%d batch_size=%d flush_interval=%s",
				len(s.queue),
				atomic.LoadUint64(&s.sent),
				atomic.LoadUint64(&s.failed),
				atomic.LoadUint64(&s.dropped),
				s.batchSize,
				s.flushInterval,
			)
		}
	}
}

func (s *TelemetrySender) postBatch(requests []TelemetryPayload) error {
	payload := BatchTelemetryPayload{Requests: requests}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal batch payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, s.batchEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("post batch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("batch endpoint returned %d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func envInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}

	return parsed
}

func envDuration(name string, defaultValue time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return defaultValue
	}

	return parsed
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

	configuredTraefikPod := os.Getenv("TRAEFIK_POD")
	appBaseDomain := strings.TrimSpace(strings.ToLower(os.Getenv("APP_BASE_DOMAIN")))
	if appBaseDomain == "" {
		appBaseDomain = "keshavstack.tech"
	}

	log.Printf("Starting traffic forwarder...")
	log.Printf("Control Plane: %s", controlPlaneURL)
	log.Printf("Traefik Namespace: %s", traefikNamespace)
	log.Printf("App Base Domain: %s", appBaseDomain)

	sender := NewTelemetrySender(controlPlaneURL)
	go sender.Run()

	log.Printf(
		"Telemetry sender configured batch_size=%d queue_size=%d flush_interval=%s endpoint=%s",
		sender.batchSize,
		cap(sender.queue),
		sender.flushInterval,
		sender.batchEndpoint,
	)

	for {
		traefikPod, containerName, err := resolveTraefikTarget(traefikNamespace, configuredTraefikPod)
		if err != nil {
			log.Printf("Failed to resolve Traefik target: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Printf("Tailing Traefik access logs from pod=%s container=%s", traefikPod, containerName)
		if err := streamTraefikLogs(traefikNamespace, traefikPod, containerName, sender, appBaseDomain); err != nil {
			log.Printf("Log stream ended: %v", err)
		} else {
			log.Printf("Log stream ended without error, reconnecting")
		}

		time.Sleep(2 * time.Second)
	}
}

func streamTraefikLogs(namespace, pod, containerName string, sender *TelemetrySender, appBaseDomain string) error {
	args := []string{"logs", "-n", namespace, "-f", pod, "--tail=20"}
	if containerName != "" {
		args = append(args, "-c", containerName)
	}

	cmd := exec.Command("kubectl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start kubectl logs: %w", err)
	}

	errLines := make(chan string, 32)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			errLines <- line
		}
		close(errLines)
	}()

	outScanner := bufio.NewScanner(stdout)
	for outScanner.Scan() {
		line := strings.TrimSpace(outScanner.Text())
		if line == "" {
			continue
		}
		processLogLine(line, sender, appBaseDomain)
	}

	if scanErr := outScanner.Err(); scanErr != nil && scanErr != io.EOF {
		_ = cmd.Wait()
		return fmt.Errorf("stdout scanner error: %w", scanErr)
	}

	waitErr := cmd.Wait()
	var stderrJoined strings.Builder
	for line := range errLines {
		log.Printf("kubectl logs stderr: %s", line)
		if stderrJoined.Len() > 0 {
			stderrJoined.WriteString(" | ")
		}
		stderrJoined.WriteString(line)
	}

	if waitErr != nil {
		if stderrJoined.Len() > 0 {
			return fmt.Errorf("kubectl logs failed: %w (%s)", waitErr, stderrJoined.String())
		}
		return fmt.Errorf("kubectl logs failed: %w", waitErr)
	}

	if stderrJoined.Len() > 0 {
		return fmt.Errorf("kubectl logs ended: %s", stderrJoined.String())
	}

	return nil
}

func resolveTraefikTarget(namespace, configuredPod string) (pod string, container string, err error) {
	if strings.TrimSpace(configuredPod) != "" {
		containerName, containerErr := resolveTraefikContainer(namespace, configuredPod)
		if containerErr != nil {
			return "", "", containerErr
		}
		return configuredPod, containerName, nil
	}

	labelSelectors := []string{
		"app.kubernetes.io/name=traefik",
		"app=traefik",
	}

	for _, selector := range labelSelectors {
		cmd := exec.Command(
			"kubectl", "get", "pods", "-n", namespace,
			"-l", selector,
			"--field-selector=status.phase=Running",
			"-o", "jsonpath={.items[0].metadata.name}",
		)
		output, cmdErr := cmd.Output()
		if cmdErr != nil {
			continue
		}

		candidate := strings.TrimSpace(string(output))
		if candidate == "" {
			continue
		}

		containerName, containerErr := resolveTraefikContainer(namespace, candidate)
		if containerErr != nil {
			continue
		}

		return candidate, containerName, nil
	}

	return "", "", fmt.Errorf("could not find running Traefik pod in namespace %s", namespace)
}

func resolveTraefikContainer(namespace, pod string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pod", "-n", namespace, pod, "-o", "jsonpath={.spec.containers[*].name}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get containers for pod %s: %w", pod, err)
	}

	containers := strings.Fields(strings.TrimSpace(string(output)))
	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found in pod %s", pod)
	}

	for _, name := range containers {
		if name == "traefik" {
			return name, nil
		}
	}

	return containers[0], nil
}

func processLogLine(line string, sender *TelemetrySender, appBaseDomain string) {
	// Parse JSON log
	var accessLog TraefikAccessLog
	if err := json.Unmarshal([]byte(line), &accessLog); err != nil {
		// Skip non-JSON lines
		return
	}

	statusCode := accessLog.OriginStatus
	if statusCode <= 0 {
		statusCode = accessLog.DownstreamStatus
	}
	if statusCode < 100 || statusCode > 599 {
		return
	}

	host := normalizeHost(accessLog.RequestHost, accessLog.RequestAddr)
	subdomain := extractSubdomain(host, appBaseDomain)
	if subdomain == "" {
		return // Not a user deployment
	}

	// Convert to telemetry payload
	payload := TelemetryPayload{
		DeploymentID:  subdomain,
		StatusCode:    statusCode,
		LatencyMs:     float64(accessLog.Duration) / 1000.0, // Convert microseconds to milliseconds
		BytesSent:     accessLog.OriginContentSize,
		BytesReceived: accessLog.RequestContentSize,
		Path:          accessLog.RequestPath,
		Timestamp:     accessLog.Time,
	}

	// Queue telemetry for async batch forwarding.
	sender.Enqueue(payload)
}

func normalizeHost(requestHost, requestAddr string) string {
	host := strings.TrimSpace(requestHost)
	if host == "" || host == "-" {
		host = strings.TrimSpace(requestAddr)
	}

	host = strings.ToLower(strings.TrimSpace(stripPort(host)))
	return host
}

func stripPort(host string) string {
	if host == "" {
		return ""
	}

	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		return parsedHost
	}

	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end > 1 {
			return host[1:end]
		}
	}

	if strings.Count(host, ":") == 1 {
		parts := strings.SplitN(host, ":", 2)
		if parts[0] != "" {
			return parts[0]
		}
	}

	return host
}

func extractSubdomain(host, appBaseDomain string) string {
	if host == "" || appBaseDomain == "" {
		return ""
	}

	if net.ParseIP(host) != nil {
		return ""
	}

	base := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(appBaseDomain)), ".")
	if base == "" {
		return ""
	}

	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return ""
	}

	subdomain := strings.TrimSuffix(host, suffix)
	if subdomain == "" || strings.Contains(subdomain, ".") {
		return ""
	}

	// Skip reserved hosts that are not user deployments.
	if subdomain == "www" || subdomain == "api" || subdomain == "control-plane" || subdomain == "self" {
		return ""
	}

	return subdomain
}
