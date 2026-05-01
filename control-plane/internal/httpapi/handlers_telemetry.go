package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"

	"github.com/gin-gonic/gin"
)

// TelemetryHandler handles incoming telemetry data from deployment proxies/middlewares
type TelemetryHandler struct {
	analyticsRepo     AnalyticsRepository
	deploymentService *service.DeploymentService
	cacheMu           sync.RWMutex
	resolveCache      map[string]resolvedDeploymentCacheEntry
}

type telemetryBatchRecorder interface {
	RecordRequestBatch(requests []domain.DeploymentRequest) error
}

type resolvedDeploymentCacheEntry struct {
	deploymentID string
	expiresAt    time.Time
}

const (
	telemetryResolveCacheTTL        = 2 * time.Minute
	telemetryResolveNegativeTTL     = 20 * time.Second
	telemetryResolveMaxCacheEntries = 10000
)

// NewTelemetryHandler creates a new telemetry handler
func NewTelemetryHandler(analyticsRepo AnalyticsRepository, deploymentService *service.DeploymentService) *TelemetryHandler {
	return &TelemetryHandler{
		analyticsRepo:     analyticsRepo,
		deploymentService: deploymentService,
		resolveCache:      make(map[string]resolvedDeploymentCacheEntry),
	}
}

// DeploymentRequestPayload represents a single HTTP request to a deployment
type DeploymentRequestPayload struct {
	DeploymentID  string  `json:"deployment_id"` // UUID or subdomain
	StatusCode    int     `json:"status_code"`
	LatencyMs     float64 `json:"latency_ms"`
	BytesSent     int64   `json:"bytes_sent"`
	BytesReceived int64   `json:"bytes_received"`
	Path          string  `json:"path"`
	Method        string  `json:"method"`
	Timestamp     string  `json:"timestamp"` // ISO 8601 format, optional
}

// RecordDeploymentRequest handles POST /api/telemetry/deployment-request
// This endpoint receives telemetry data from Traefik middleware or deployment proxie
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

	// Validate required fields
	if payload.DeploymentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deployment_id is required"})
		return
	}
	if payload.StatusCode < 100 || payload.StatusCode > 599 {
		logs.Warnf("telemetry", "dropping telemetry with invalid status_code=%d deployment_id=%s", payload.StatusCode, payload.DeploymentID)
		c.JSON(http.StatusAccepted, gin.H{"status": "dropped", "reason": "invalid_status_code"})
		return
	}

	deploymentID, ok := h.resolveTelemetryDeploymentID(payload.DeploymentID)
	if !ok {
		logs.Warnf("telemetry", "dropping telemetry with unresolved deployment identifier=%s", payload.DeploymentID)
		c.JSON(http.StatusAccepted, gin.H{"status": "dropped", "reason": "unknown_deployment"})
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
		deploymentID,
		payload.StatusCode,
		payload.LatencyMs/1000.0, // Convert ms to seconds for Prometheus
		payload.BytesSent,
		payload.BytesReceived,
	)

	// Record in database if analytics repository is available
	if h.analyticsRepo != nil {
		req := domain.DeploymentRequest{
			DeploymentID:  deploymentID,
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
	dropped := 0

	resolvedInBatch := make(map[string]string)
	unresolvedInBatch := make(map[string]struct{})
	dbRequests := make([]domain.DeploymentRequest, 0, len(payload.Requests))

	for _, req := range payload.Requests {
		identifier := strings.TrimSpace(req.DeploymentID)
		if identifier == "" {
			dropped++
			continue
		}
		if req.StatusCode < 100 || req.StatusCode > 599 {
			dropped++
			continue
		}

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

		if _, seen := unresolvedInBatch[identifier]; seen {
			dropped++
			continue
		}

		deploymentID, seen := resolvedInBatch[identifier]
		if !seen {
			var ok bool
			deploymentID, ok = h.resolveTelemetryDeploymentID(identifier)
			if !ok {
				unresolvedInBatch[identifier] = struct{}{}
				dropped++
				continue
			}
			resolvedInBatch[identifier] = deploymentID
		}

		// Update Prometheus metrics
		telemetry.ObserveDeploymentRequest(
			deploymentID,
			req.StatusCode,
			req.LatencyMs/1000.0,
			req.BytesSent,
			req.BytesReceived,
		)

		dbReq := domain.DeploymentRequest{
			DeploymentID:  deploymentID,
			Timestamp:     timestamp,
			StatusCode:    req.StatusCode,
			LatencyMs:     req.LatencyMs,
			BytesSent:     req.BytesSent,
			BytesReceived: req.BytesReceived,
			Path:          req.Path,
		}

		dbRequests = append(dbRequests, dbReq)
	}

	if h.analyticsRepo != nil {
		if batchRecorder, ok := h.analyticsRepo.(telemetryBatchRecorder); ok {
			if err := batchRecorder.RecordRequestBatch(dbRequests); err != nil {
				failed += len(dbRequests)
				logs.Errorf("telemetry", "failed to record request batch size=%d: %v", len(dbRequests), err)
			} else {
				recorded += len(dbRequests)
			}
		} else {
			for _, dbReq := range dbRequests {
				if err := h.analyticsRepo.RecordRequest(dbReq); err != nil {
					failed++
				} else {
					recorded++
				}
			}
		}
	} else {
		recorded += len(dbRequests)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "batch_processed",
		"recorded": recorded,
		"failed":   failed,
		"dropped":  dropped,
		"total":    len(payload.Requests),
	})
}

func (h *TelemetryHandler) resolveTelemetryDeploymentID(identifier string) (string, bool) {
	deploymentID := strings.TrimSpace(identifier)
	if deploymentID == "" {
		return "", false
	}

	if cachedID, ok := h.getCachedDeploymentID(deploymentID); ok {
		if cachedID == "" {
			return "", false
		}
		return cachedID, true
	}

	if h.deploymentService == nil {
		h.setCachedDeploymentID(deploymentID, deploymentID, telemetryResolveCacheTTL)
		return deploymentID, true
	}

	resolved := strings.TrimSpace(h.deploymentService.ResolveDeploymentID(deploymentID))
	if resolved == "" {
		h.setCachedDeploymentID(deploymentID, "", telemetryResolveNegativeTTL)
		return "", false
	}

	if _, err := h.deploymentService.GetDeployment(resolved); err != nil {
		h.setCachedDeploymentID(deploymentID, "", telemetryResolveNegativeTTL)
		return "", false
	}

	h.setCachedDeploymentID(deploymentID, resolved, telemetryResolveCacheTTL)
	h.setCachedDeploymentID(resolved, resolved, telemetryResolveCacheTTL)

	return resolved, true
}

func (h *TelemetryHandler) getCachedDeploymentID(identifier string) (string, bool) {
	now := time.Now()

	h.cacheMu.RLock()
	entry, ok := h.resolveCache[identifier]
	h.cacheMu.RUnlock()

	if !ok {
		return "", false
	}
	if now.After(entry.expiresAt) {
		h.cacheMu.Lock()
		delete(h.resolveCache, identifier)
		h.cacheMu.Unlock()
		return "", false
	}

	return entry.deploymentID, true
}

func (h *TelemetryHandler) setCachedDeploymentID(identifier, deploymentID string, ttl time.Duration) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()

	if len(h.resolveCache) >= telemetryResolveMaxCacheEntries {
		h.resolveCache = make(map[string]resolvedDeploymentCacheEntry, telemetryResolveMaxCacheEntries)
	}

	h.resolveCache[identifier] = resolvedDeploymentCacheEntry{
		deploymentID: deploymentID,
		expiresAt:    time.Now().Add(ttl),
	}
}
