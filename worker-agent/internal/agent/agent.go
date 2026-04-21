package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"worker-agent/internal/config"
	"worker-agent/internal/executor"
	"worker-agent/internal/metrics"
)

type Agent struct {
	cfg         *config.Config
	executor    *executor.JobExecutor
	client      *http.Client
	activeJobs  int
	podsManaged int
}

func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:      cfg,
		executor: executor.New(cfg.Runtime),
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *Agent) Start(ctx context.Context) {
	log.Printf("Starting worker agent: %s (%s)", a.cfg.Worker.Name, a.cfg.Worker.ID)

	// Register with control-plane
	if err := a.register(); err != nil {
		log.Fatalf("Failed to register with control-plane: %v", err)
	}

	// Start heartbeat loop
	go a.heartbeatLoop(ctx)

	// Start job polling loop
	a.pollJobs(ctx)
}

func (a *Agent) register() error {
	payload := map[string]interface{}{
		"worker_id":    a.cfg.Worker.ID,
		"name":         a.cfg.Worker.Name,
		"tailscale_ip": a.cfg.Worker.TailscaleIP,
		"hostname":     getHostname(),
		"capabilities": map[string]interface{}{
			"runtime":             a.cfg.Runtime.Type,
			"memory_gb":           a.cfg.Capabilities.MemoryGB,
			"cpu_cores":           a.cfg.Capabilities.CPUCores,
			"max_concurrent_jobs": a.cfg.Worker.MaxConcurrentJobs,
			"supported_packages":  a.cfg.Capabilities.SupportedPackages,
		},
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/workers/register", a.cfg.ControlPlane.URL)

	resp, err := a.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed: %s", string(body))
	}

	log.Printf("Successfully registered with control-plane")
	return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.sendHeartbeat()
		}
	}
}

func (a *Agent) sendHeartbeat() {
	status := "idle"
	if a.activeJobs > 0 {
		status = "busy"
	}

	payload := map[string]interface{}{
		"status":       status,
		"current_jobs": a.activeJobs,
	}

	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/workers/%s/heartbeat", a.cfg.ControlPlane.URL, a.cfg.Worker.ID)

	resp, err := a.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
		metrics.RecordHeartbeat(false)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Heartbeat failed with status %d", resp.StatusCode)
		metrics.RecordHeartbeat(false)
	} else {
		metrics.RecordHeartbeat(true)
	}

	// Update metrics
	metrics.SetActiveJobs(a.activeJobs)
	metrics.SetPodsManaged(a.podsManaged)
}

func (a *Agent) pollJobs(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.claimAndExecuteJob(ctx)
		}
	}
}

func (a *Agent) claimAndExecuteJob(ctx context.Context) {
	// Claim job from control-plane
	url := fmt.Sprintf("%s/api/workers/%s/claim-job", a.cfg.ControlPlane.URL, a.cfg.Worker.ID)
	resp, err := a.client.Get(url)
	if err != nil {
		log.Printf("Failed to claim job: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Job *json.RawMessage `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	if result.Job == nil {
		// No jobs available
		return
	}

	// Parse job
	var job map[string]interface{}
	if err := json.Unmarshal(*result.Job, &job); err != nil {
		log.Printf("Failed to parse job: %v", err)
		return
	}

	jobID, ok := job["job_id"].(string)
	if !ok {
		log.Printf("Job missing job_id field")
		return
	}

	deploymentID, _ := job["deployment_id"].(string)

	log.Printf("Claimed job: %s", jobID)

	// Track job execution
	a.activeJobs++
	metrics.SetActiveJobs(a.activeJobs)
	startTime := time.Now()

	// Execute job
	if err := a.executor.Execute(ctx, job); err != nil {
		log.Printf("Job execution failed: %v", err)
		a.activeJobs--
		duration := time.Since(startTime).Seconds()
		metrics.RecordJobCompletion("failed", duration)
		metrics.SetActiveJobs(a.activeJobs)
		a.reportJobFailed(jobID, deploymentID, err.Error())
		return
	}

	log.Printf("Job completed successfully: %s", jobID)
	a.activeJobs--
	duration := time.Since(startTime).Seconds()
	metrics.RecordJobCompletion("success", duration)
	metrics.SetActiveJobs(a.activeJobs)
	a.reportJobComplete(jobID, deploymentID)
}

func (a *Agent) reportJobComplete(jobID, deploymentID string) {
	payload := map[string]string{"job_id": jobID, "deployment_id": deploymentID}
	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/workers/%s/job-complete", a.cfg.ControlPlane.URL, a.cfg.Worker.ID)

	resp, err := a.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to report job completion: %v", err)
		return
	}
	defer resp.Body.Close()
}

func (a *Agent) reportJobFailed(jobID, deploymentID, errorMsg string) {
	payload := map[string]string{
		"job_id":        jobID,
		"deployment_id": deploymentID,
		"error":         errorMsg,
	}
	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/api/workers/%s/job-failed", a.cfg.ControlPlane.URL, a.cfg.Worker.ID)

	resp, err := a.client.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Failed to report job failure: %v", err)
		return
	}
	defer resp.Body.Close()
}

func getHostname() string {
	hostname, _ := os.Hostname()
	return hostname
}
