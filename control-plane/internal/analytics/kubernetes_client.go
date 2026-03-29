package analytics

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

// KubernetesClient queries Kubernetes for pod and deployment metrics
type KubernetesClient struct {
	namespace   string
	kubectl     string
	cache       *metricsCache
	cacheTTL    time.Duration
	mu          sync.RWMutex
}

// metricsCache stores cached pod metrics
type metricsCache struct {
	data map[string]*cachedMetrics
	mu   sync.RWMutex
}

type cachedMetrics struct {
	pods      []domain.PodMetrics
	resources domain.ResourceAllocation
	timestamp time.Time
}

// NewKubernetesClient creates a new Kubernetes client
func NewKubernetesClient(namespace, kubectl string) *KubernetesClient {
	if namespace == "" {
		namespace = "meshvpn-apps"
	}
	if kubectl == "" {
		kubectl = "kubectl"
	}

	return &KubernetesClient{
		namespace: namespace,
		kubectl:   kubectl,
		cache: &metricsCache{
			data: make(map[string]*cachedMetrics),
		},
		cacheTTL: 12 * time.Second, // 12 second cache TTL
	}
}

// GetPodMetrics retrieves detailed metrics for all pods of a deployment
func (k *KubernetesClient) GetPodMetrics(deploymentID string) ([]domain.PodMetrics, error) {
	// Check cache first
	if cached := k.getFromCache(deploymentID); cached != nil {
		logs.Debugf("k8s-client", "cache hit for deployment_id=%s", deploymentID)
		return cached.pods, nil
	}

	logs.Debugf("k8s-client", "cache miss for deployment_id=%s, querying K8s", deploymentID)

	// Get pod list with status
	pods, err := k.getPodList(deploymentID)
	if err != nil {
		return nil, err
	}

	if len(pods) == 0 {
		return []domain.PodMetrics{}, nil
	}

	// Enrich with resource usage
	if err := k.enrichPodMetrics(deploymentID, pods); err != nil {
		logs.Debugf("k8s-client", "failed to enrich pod metrics for %s: %v", deploymentID, err)
		// Continue with pods without resource metrics
	}

	return pods, nil
}

// GetResourceAllocation gets the requested/limit values from deployment spec
func (k *KubernetesClient) GetResourceAllocation(deploymentID string) (domain.ResourceAllocation, error) {
	// Check cache first
	if cached := k.getFromCache(deploymentID); cached != nil {
		return cached.resources, nil
	}

	deploymentName := "app-" + deploymentID

	// Query deployment spec for resource requests/limits
	cmd := exec.Command(k.kubectl, "-n", k.namespace, "get", "deployment", deploymentName,
		"-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return domain.ResourceAllocation{}, fmt.Errorf("get deployment spec: %w", err)
	}

	var deployment struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Resources struct {
							Requests map[string]string `json:"requests"`
							Limits   map[string]string `json:"limits"`
						} `json:"resources"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(output, &deployment); err != nil {
		return domain.ResourceAllocation{}, fmt.Errorf("parse deployment spec: %w", err)
	}

	allocation := domain.ResourceAllocation{}

	if len(deployment.Spec.Template.Spec.Containers) > 0 {
		resources := deployment.Spec.Template.Spec.Containers[0].Resources

		// Parse CPU requests
		if cpuReq, ok := resources.Requests["cpu"]; ok {
			milli, err := parseCPUToMilli(cpuReq)
			if err == nil {
				allocation.CPURequested = int(milli)
			}
		}

		// Parse CPU limits
		if cpuLim, ok := resources.Limits["cpu"]; ok {
			milli, err := parseCPUToMilli(cpuLim)
			if err == nil {
				allocation.CPULimit = int(milli)
			}
		}

		// Parse memory requests
		if memReq, ok := resources.Requests["memory"]; ok {
			mb, err := parseMemoryToMB(memReq)
			if err == nil {
				allocation.MemoryRequested = int(mb)
			}
		}

		// Parse memory limits
		if memLim, ok := resources.Limits["memory"]; ok {
			mb, err := parseMemoryToMB(memLim)
			if err == nil {
				allocation.MemoryLimit = int(mb)
			}
		}
	}

	return allocation, nil
}

// getPodList retrieves pod names and status
func (k *KubernetesClient) getPodList(deploymentID string) ([]domain.PodMetrics, error) {
	deploymentName := "app-" + deploymentID

	cmd := exec.Command(k.kubectl, "-n", k.namespace, "get", "pods",
		"-l", "app="+deploymentName, "-o", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get pods: %w", err)
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name              string    `json:"name"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready        bool `json:"ready"`
					RestartCount int  `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}

	if err := json.Unmarshal(output, &podList); err != nil {
		return nil, fmt.Errorf("parse pod list: %w", err)
	}

	pods := make([]domain.PodMetrics, 0, len(podList.Items))
	for _, item := range podList.Items {
		ready := false
		restarts := 0
		if len(item.Status.ContainerStatuses) > 0 {
			ready = item.Status.ContainerStatuses[0].Ready
			restarts = item.Status.ContainerStatuses[0].RestartCount
		}

		age := time.Since(item.Metadata.CreationTimestamp)
		ageStr := formatDuration(age)

		pods = append(pods, domain.PodMetrics{
			PodName:   item.Metadata.Name,
			Status:    item.Status.Phase,
			Ready:     ready,
			Restarts:  restarts,
			Age:       ageStr,
			CreatedAt: item.Metadata.CreationTimestamp,
		})
	}

	return pods, nil
}

// enrichPodMetrics enriches pod metrics with CPU/memory usage
func (k *KubernetesClient) enrichPodMetrics(deploymentID string, pods []domain.PodMetrics) error {
	deploymentName := "app-" + deploymentID

	cmd := exec.Command(k.kubectl, "-n", k.namespace, "top", "pod",
		"-l", "app="+deploymentName, "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("kubectl top pod: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	usageMap := make(map[string]struct {
		cpu    int64
		memory float64
	})

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		podName := fields[0]
		cpuMilli, err := parseCPUToMilli(fields[1])
		if err != nil {
			continue
		}

		memMB, err := parseMemoryToMB(fields[2])
		if err != nil {
			continue
		}

		usageMap[podName] = struct {
			cpu    int64
			memory float64
		}{cpu: cpuMilli, memory: memMB}
	}

	// Update pods with usage data
	for i := range pods {
		if usage, ok := usageMap[pods[i].PodName]; ok {
			pods[i].CPUUsageMilli = usage.cpu
			pods[i].MemoryUsageMB = usage.memory
		}
	}

	return nil
}

// GetPodMetricsWithResources retrieves both pod metrics and resource allocation
func (k *KubernetesClient) GetPodMetricsWithResources(deploymentID string) ([]domain.PodMetrics, domain.ResourceAllocation, error) {
	// Check cache
	if cached := k.getFromCache(deploymentID); cached != nil {
		return cached.pods, cached.resources, nil
	}

	// Fetch fresh data
	pods, err := k.GetPodMetrics(deploymentID)
	if err != nil {
		return nil, domain.ResourceAllocation{}, err
	}

	resources, err := k.GetResourceAllocation(deploymentID)
	if err != nil {
		logs.Debugf("k8s-client", "failed to get resource allocation for %s: %v", deploymentID, err)
		// Continue with empty resources
	}

	// Calculate aggregated usage
	var totalCPU int64
	var totalMem float64
	for _, pod := range pods {
		if pod.Status == "Running" && pod.Ready {
			totalCPU += pod.CPUUsageMilli
			totalMem += pod.MemoryUsageMB
		}
	}

	resources.CPUUsageMilli = totalCPU
	resources.MemoryUsageMB = totalMem

	// Calculate usage percentages
	if resources.CPURequested > 0 {
		resources.CPUUsagePercent = (float64(totalCPU) / float64(resources.CPURequested)) * 100.0
	}
	if resources.MemoryRequested > 0 {
		resources.MemoryUsagePercent = (totalMem / float64(resources.MemoryRequested)) * 100.0
	}

	// Store in cache
	k.setCache(deploymentID, pods, resources)

	return pods, resources, nil
}

// getFromCache retrieves cached metrics if still valid
func (k *KubernetesClient) getFromCache(deploymentID string) *cachedMetrics {
	k.cache.mu.RLock()
	defer k.cache.mu.RUnlock()

	cached, ok := k.cache.data[deploymentID]
	if !ok {
		return nil
	}

	// Check if cache is still valid
	if time.Since(cached.timestamp) > k.cacheTTL {
		return nil
	}

	return cached
}

// setCache stores metrics in cache
func (k *KubernetesClient) setCache(deploymentID string, pods []domain.PodMetrics, resources domain.ResourceAllocation) {
	k.cache.mu.Lock()
	defer k.cache.mu.Unlock()

	k.cache.data[deploymentID] = &cachedMetrics{
		pods:      pods,
		resources: resources,
		timestamp: time.Now(),
	}

	// Cleanup old cache entries (older than 1 minute)
	cutoff := time.Now().Add(-1 * time.Minute)
	for id, cached := range k.cache.data {
		if cached.timestamp.Before(cutoff) {
			delete(k.cache.data, id)
		}
	}
}

// formatDuration formats a duration into human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return fmt.Sprintf("%dd", days)
}
