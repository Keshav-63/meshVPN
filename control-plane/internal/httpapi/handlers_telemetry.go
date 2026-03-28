package httpapi

import (
	"net/http"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"

	"github.com/gin-gonic/gin"
)

// TelemetryHandler handles incoming telemetry data from deployment proxies/middlewares
type TelemetryHandler struct {
	analyticsRepo AnalyticsRepository
}

// NewTelemetryHandler creates a new telemetry handler
func NewTelemetryHandler(analyticsRepo AnalyticsRepository) *TelemetryHandler {
	return &TelemetryHandler{
		analyticsRepo: analyticsRepo,
	}
}

// DeploymentRequestPayload represents a single HTTP request to a deployment
type DeploymentRequestPayload struct {
	DeploymentID  string  `json:"deployment_id" binding:"required"`
	StatusCode    int     `json:"status_code" binding:"required"`
	LatencyMs     float64 `json:"latency_ms" binding:"required"`
	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	Path          string  `json:"path"`
	Method        string  `json:"method"`
	Timestamp     string  `json:"timestamp"` // ISO 8601 format, optional
}

// RecordDeploymentRequest handles POST /api/telemetry/deployment-request
// This endpoint receives telemetry data from Traefik middleware or deployment proxies
//
// @Summary Record deployment HTTP request metrics
// @Description Records metrics for a single HTTP request to a user deployment
// @Tags Telemetry
// @Accept json
// @Produce json
// @Param request body DeploymentRequestPayload true "Request metrics"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/telemetry/deployment-request [post]
func (h *TelemetryHandler) RecordDeploymentRequest(c *gin.Context) {
	var payload DeploymentRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		logs.Errorf("telemetry", "invalid payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Parse timestamp or use current time
	var timestamp time.Time
	if payload.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339, payload.Timestamp)
		if err != nil {
			logs.Debugf("telemetry", "failed to parse timestamp, using current time: %v", err)
			timestamp = time.Now().UTC()
		} else {
			timestamp = parsed.UTC()
		}
	} else {
		timestamp = time.Now().UTC()
	}

	// Update Prometheus metrics
	telemetry.ObserveDeploymentRequest(
		payload.DeploymentID,
		payload.StatusCode,
		payload.LatencyMs/1000.0, // Convert ms to seconds for Prometheus
		payload.BytesSent,
		payload.BytesReceived,
	)

	// Record in database if analytics repository is available
	if h.analyticsRepo != nil {
		req := domain.DeploymentRequest{
			DeploymentID:  payload.DeploymentID,
			Timestamp:     timestamp,
			StatusCode:    payload.StatusCode,
			LatencyMs:     payload.LatencyMs,
			BytesSent:     payload.BytesSent,
			BytesReceived: payload.BytesReceived,
			Path:          payload.Path,
		}

		if err := h.analyticsRepo.RecordRequest(req); err != nil {
			logs.Errorf("telemetry", "failed to record request in database: %v", err)
			// Don't fail the request - telemetry should be fire-and-forget
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "recorded"})
}

// BatchDeploymentRequestPayload represents multiple deployment requests
type BatchDeploymentRequestPayload struct {
	Requests []DeploymentRequestPayload `json:"requests" binding:"required"`
}

// RecordDeploymentRequestBatch handles POST /api/telemetry/deployment-request/batch
// This endpoint receives batched telemetry data for better performance
//
// @Summary Record multiple deployment HTTP request metrics
// @Description Records metrics for multiple HTTP requests to user deployments in a batch
// @Tags Telemetry
// @Accept json
// @Produce json
// @Param requests body BatchDeploymentRequestPayload true "Batch of request metrics"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/telemetry/deployment-request/batch [post]
func (h *TelemetryHandler) RecordDeploymentRequestBatch(c *gin.Context) {
	var payload BatchDeploymentRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		logs.Errorf("telemetry", "invalid batch payload: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	recorded := 0
	failed := 0

	for _, req := range payload.Requests {
		// Parse timestamp or use current time
		var timestamp time.Time
		if req.Timestamp != "" {
			parsed, err := time.Parse(time.RFC3339, req.Timestamp)
			if err != nil {
				timestamp = time.Now().UTC()
			} else {
				timestamp = parsed.UTC()
			}
		} else {
			timestamp = time.Now().UTC()
		}

		// Update Prometheus metrics
		telemetry.ObserveDeploymentRequest(
			req.DeploymentID,
			req.StatusCode,
			req.LatencyMs/1000.0,
			req.BytesSent,
			req.BytesReceived,
		)

		// Record in database
		if h.analyticsRepo != nil {
			dbReq := domain.DeploymentRequest{
				DeploymentID:  req.DeploymentID,
				Timestamp:     timestamp,
				StatusCode:    req.StatusCode,
				LatencyMs:     req.LatencyMs,
				BytesSent:     req.BytesSent,
				BytesReceived: req.BytesReceived,
				Path:          req.Path,
			}

			if err := h.analyticsRepo.RecordRequest(dbReq); err != nil {
				failed++
			} else {
				recorded++
			}
		} else {
			recorded++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "batch_processed",
		"recorded": recorded,
		"failed":   failed,
		"total":    len(payload.Requests),
	})
}
