package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
)

type AnalyticsRepository interface {
	GetMetrics(deploymentID string) (domain.DeploymentMetrics, error)
	RecordRequest(req domain.DeploymentRequest) error
}

type AnalyticsHandler struct {
	deploymentService *service.DeploymentService
	analyticsRepo     AnalyticsRepository
}

func NewAnalyticsHandler(deploymentService *service.DeploymentService, analyticsRepo AnalyticsRepository) *AnalyticsHandler {
	return &AnalyticsHandler{
		deploymentService: deploymentService,
		analyticsRepo:     analyticsRepo,
	}
}

// GetUserAnalytics returns aggregated analytics for the authenticated user.
// @Summary      Get user analytics summary
// @Description  Returns aggregate and per-deployment analytics for the authenticated user.
// @Tags         Analytics
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "User analytics summary"
// @Failure      401  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /user/analytics [get]
func (h *AnalyticsHandler) GetUserAnalytics(c *gin.Context) {
	var userID string
	if userVal, exists := c.Get("auth.user"); exists {
		user := userVal.(domain.User)
		userID = user.UserID
	}
	if userID == "" {
		userID = strings.TrimSpace(c.GetString("auth.sub"))
	}

	deployments := h.deploymentService.ListDeployments()
	if userID != "" {
		deployments = h.deploymentService.ListDeploymentsByUser(userID)
	}

	items := make([]gin.H, 0, len(deployments))
	running := 0
	failed := 0
	queued := 0

	var totalRequests int64
	var lastHourRequests int64
	var last24hRequests int64
	var totalBandwidthSent int64
	var totalBandwidthRecv int64
	var currentPods int
	var desiredPods int

	for _, dep := range deployments {
		metrics, err := h.analyticsRepo.GetMetrics(dep.DeploymentID)
		if err != nil {
			logs.Errorf("analytics", "failed to load user metrics deployment_id=%s: %v", dep.DeploymentID, err)
			continue
		}

		switch dep.Status {
		case "running":
			running++
		case "failed":
			failed++
		case "queued", "deploying":
			queued++
		}

		totalRequests += metrics.RequestCountTotal
		lastHourRequests += metrics.RequestCount1h
		last24hRequests += metrics.RequestCount24h
		totalBandwidthSent += metrics.BandwidthSentBytes
		totalBandwidthRecv += metrics.BandwidthRecvBytes
		currentPods += metrics.CurrentPods
		desiredPods += metrics.DesiredPods

		items = append(items, gin.H{
			"deployment_id": dep.DeploymentID,
			"subdomain":     dep.Subdomain,
			"url":           dep.URL,
			"status":        dep.Status,
			"package":       normalizePackage(dep.Package),
			"metrics": gin.H{
				"requests_total":       metrics.RequestCountTotal,
				"requests_last_hour":   metrics.RequestCount1h,
				"requests_last_24h":    metrics.RequestCount24h,
				"bandwidth_sent_bytes": metrics.BandwidthSentBytes,
				"bandwidth_recv_bytes": metrics.BandwidthRecvBytes,
				"current_pods":         metrics.CurrentPods,
				"desired_pods":         metrics.DesiredPods,
				"cpu_usage_percent":    metrics.CPUUsagePercent,
				"memory_usage_mb":      metrics.MemoryUsageMB,
				"latency_p50_ms":       metrics.LatencyP50Ms,
				"latency_p90_ms":       metrics.LatencyP90Ms,
				"latency_p99_ms":       metrics.LatencyP99Ms,
				"last_updated":         metrics.LastUpdated,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id": userID,
		"summary": gin.H{
			"deployments_total":    len(deployments),
			"deployments_running":  running,
			"deployments_failed":   failed,
			"deployments_queued":   queued,
			"requests_total":       totalRequests,
			"requests_last_hour":   lastHourRequests,
			"requests_last_24h":    last24hRequests,
			"bandwidth_sent_bytes": totalBandwidthSent,
			"bandwidth_recv_bytes": totalBandwidthRecv,
			"pods_current":         currentPods,
			"pods_desired":         desiredPods,
		},
		"deployments": items,
	})
}

func normalizePackage(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return "small"
	}

	return pkg
}

// GetAnalytics returns current metrics for a deployment (GET /deployments/:id/analytics)
// @Summary      Get deployment analytics
// @Description  Get aggregated metrics for a specific deployment (backward compatible)
// @Tags         Analytics
// @Accept       json
// @Produce      json
// @Param        id   path      string  true  "Deployment ID"
// @Success      200  {object}  map[string]interface{}  "Deployment analytics with metrics"
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /deployments/{id}/analytics [get]
func (h *AnalyticsHandler) GetAnalytics(c *gin.Context) {
	deploymentID := c.Param("id")

	// Verify deployment exists and user has access
	deployment, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Check ownership if user context available
	if userVal, exists := c.Get("auth.user"); exists {
		user := userVal.(domain.User)
		if deployment.UserID != "" && deployment.UserID != user.UserID {
			logs.Errorf("analytics", "unauthorized access attempt user=%s deployment=%s owner=%s",
				user.UserID, deploymentID, deployment.UserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}

	// Get metrics from analytics repository
	metrics, err := h.analyticsRepo.GetMetrics(deploymentID)
	if err != nil {
		logs.Errorf("analytics", "failed to get metrics deployment_id=%s: %v", deploymentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load metrics"})
		return
	}

	// Build response
	response := gin.H{
		"deployment_id": deploymentID,
		"deployment": gin.H{
			"repo":         deployment.Repo,
			"subdomain":    deployment.Subdomain,
			"url":          deployment.URL,
			"package":      deployment.Package,
			"status":       deployment.Status,
			"scaling_mode": deployment.ScalingMode,
			"min_replicas": deployment.MinReplicas,
			"max_replicas": deployment.MaxReplicas,
			"started_at":   deployment.StartedAt,
		},
		"metrics": gin.H{
			"requests": gin.H{
				"total":      metrics.RequestCountTotal,
				"last_hour":  metrics.RequestCount1h,
				"last_24h":   metrics.RequestCount24h,
				"per_second": metrics.RequestsPerSecond,
			},
			"latency": gin.H{
				"p50_ms": metrics.LatencyP50Ms,
				"p90_ms": metrics.LatencyP90Ms,
				"p99_ms": metrics.LatencyP99Ms,
			},
			"bandwidth": gin.H{
				"sent_bytes":     metrics.BandwidthSentBytes,
				"received_bytes": metrics.BandwidthRecvBytes,
			},
			"pods": gin.H{
				"current": metrics.CurrentPods,
				"desired": metrics.DesiredPods,
			},
			"resources": gin.H{
				"cpu_usage_percent": metrics.CPUUsagePercent,
				"memory_usage_mb":   metrics.MemoryUsageMB,
			},
			"last_updated": metrics.LastUpdated,
		},
	}

	c.JSON(http.StatusOK, response)
}

// StreamAnalytics provides real-time metrics via Server-Sent Events (GET /deployments/:id/analytics/stream)
// @Summary      Stream live deployment metrics
// @Description  Stream real-time metrics updates every 5 seconds via Server-Sent Events
// @Tags         Analytics
// @Accept       json
// @Produce      text/event-stream
// @Param        id     path      string  true   "Deployment ID"
// @Param        token  query     string  false  "JWT token (SSE doesn't support headers)"
// @Success      200    {string}  string  "SSE stream"
// @Failure      401    {object}  ErrorResponse
// @Failure      403    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Security     BearerAuth
// @Router       /deployments/{id}/analytics/stream [get]
func (h *AnalyticsHandler) StreamAnalytics(c *gin.Context) {
	deploymentID := c.Param("id")

	// Verify deployment exists and user has access
	deployment, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	// Check ownership if user context available
	if userVal, exists := c.Get("auth.user"); exists {
		user := userVal.(domain.User)
		if deployment.UserID != "" && deployment.UserID != user.UserID {
			logs.Errorf("analytics", "unauthorized SSE access attempt user=%s deployment=%s owner=%s",
				user.UserID, deploymentID, deployment.UserID)
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
	}

	logs.Infof("analytics", "SSE stream started deployment_id=%s", deploymentID)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	// Create ticker for periodic updates (every 5 seconds)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Send initial data immediately
	h.sendMetricsSSE(c, deploymentID)

	for {
		select {
		case <-c.Request.Context().Done():
			logs.Infof("analytics", "SSE stream closed deployment_id=%s", deploymentID)
			return
		case <-ticker.C:
			h.sendMetricsSSE(c, deploymentID)
		}
	}
}

// sendMetricsSSE sends a single metrics update via SSE
func (h *AnalyticsHandler) sendMetricsSSE(c *gin.Context, deploymentID string) {
	metrics, err := h.analyticsRepo.GetMetrics(deploymentID)
	if err != nil {
		logs.Errorf("analytics", "SSE metrics fetch failed deployment_id=%s: %v", deploymentID, err)
		// Send error event
		fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\": \"failed to fetch metrics\"}\n\n")
		c.Writer.Flush()
		return
	}

	// Build metrics payload
	payload := gin.H{
		"deployment_id": deploymentID,
		"timestamp":     time.Now().Unix(),
		"requests": gin.H{
			"total":      metrics.RequestCountTotal,
			"last_hour":  metrics.RequestCount1h,
			"last_24h":   metrics.RequestCount24h,
			"per_second": metrics.RequestsPerSecond,
		},
		"latency": gin.H{
			"p50_ms": metrics.LatencyP50Ms,
			"p90_ms": metrics.LatencyP90Ms,
			"p99_ms": metrics.LatencyP99Ms,
		},
		"bandwidth": gin.H{
			"sent_bytes":     metrics.BandwidthSentBytes,
			"received_bytes": metrics.BandwidthRecvBytes,
		},
		"pods": gin.H{
			"current": metrics.CurrentPods,
			"desired": metrics.DesiredPods,
		},
		"resources": gin.H{
			"cpu_usage_percent": metrics.CPUUsagePercent,
			"memory_usage_mb":   metrics.MemoryUsageMB,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		logs.Errorf("analytics", "SSE marshal failed: %v", err)
		return
	}

	// Send SSE message
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	c.Writer.Flush()

	logs.Debugf("analytics", "SSE update sent deployment_id=%s pods=%d/%d requests_1h=%d",
		deploymentID, metrics.CurrentPods, metrics.DesiredPods, metrics.RequestCount1h)
}
