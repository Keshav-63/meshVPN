package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TraefikAccessLog represents a Traefik access log entry
type TraefikAccessLog struct {
	ClientHost         string `json:"ClientHost"`
	RequestHost        string `json:"RequestHost"`
	RequestMethod      string `json:"RequestMethod"`
	RequestPath        string `json:"RequestPath"`
	OriginStatus       int    `json:"OriginStatus"`
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

	log.Printf("Starting traffic forwarder...")
	log.Printf("Control Plane: %s", controlPlaneURL)
	log.Printf("Traefik Namespace: %s", traefikNamespace)

	for {
		traefikPod, containerName, err := resolveTraefikTarget(traefikNamespace, configuredTraefikPod)
		if err != nil {
			log.Printf("Failed to resolve Traefik target: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		log.Printf("Tailing Traefik access logs from pod=%s container=%s", traefikPod, containerName)
		if err := streamTraefikLogs(traefikNamespace, traefikPod, containerName, controlPlaneURL); err != nil {
			log.Printf("Log stream ended: %v", err)
		} else {
			log.Printf("Log stream ended without error, reconnecting")
		}

		time.Sleep(2 * time.Second)
	}
}

func streamTraefikLogs(namespace, pod, containerName, controlPlaneURL string) error {
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
		processLogLine(line, controlPlaneURL)
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
