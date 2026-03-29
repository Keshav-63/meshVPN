package httpapi

import (
	"net/http"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
)

// DeploymentDetailsHandler handles comprehensive deployment information requests
type DeploymentDetailsHandler struct {
	detailsService    *service.DeploymentDetailsService
	deploymentService *service.DeploymentService
}

// NewDeploymentDetailsHandler creates a new deployment details handler
func NewDeploymentDetailsHandler(
	detailsService *service.DeploymentDetailsService,
	deploymentService *service.DeploymentService,
) *DeploymentDetailsHandler {
	return &DeploymentDetailsHandler{
		detailsService:    detailsService,
		deploymentService: deploymentService,
	}
}

// GetDeploymentDetails godoc
// @Summary      Get comprehensive deployment details
// @Description  Retrieve complete deployment information including config, metrics, pods, and resources
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Deployment ID"
// @Success      200  {object}  domain.DeploymentDetails
// @Failure      401  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /deployments/{id} [get]
func (h *DeploymentDetailsHandler) GetDeploymentDetails(c *gin.Context) {
	deploymentID := c.Param("id")
	logs.Debugf("http", "comprehensive deployment details request deployment_id=%s", deploymentID)

	// Get deployment details
	details, err := h.detailsService.GetDeploymentDetails(deploymentID)
	if err != nil {
		logs.Errorf("http", "failed to get deployment details deployment_id=%s: %v", deploymentID, err)
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "deployment not found"})
		return
	}

	// Check user authorization
	user, userExists := c.Get("auth.user")
	if userExists {
		actualUser := user.(domain.User)
		if details.Deployment.UserID != "" && details.Deployment.UserID != actualUser.UserID {
			logs.Errorf("http", "user %s attempted to access deployment %s owned by %s",
				actualUser.UserID, deploymentID, details.Deployment.UserID)
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "access denied"})
			return
		}
	}

	// Build comprehensive response
	response := gin.H{
		"deployment": gin.H{
			"deployment_id": details.Deployment.DeploymentID,
			"repo":          details.Deployment.Repo,
			"subdomain":     details.Deployment.Subdomain,
			"url":           details.Deployment.URL,
			"status":        details.Deployment.Status,
			"package":       details.Deployment.Package,
			"port":          details.Deployment.Port,
			"scaling_mode":  details.Deployment.ScalingMode,
			"min_replicas":  details.Deployment.MinReplicas,
			"max_replicas":  details.Deployment.MaxReplicas,
			"cpu_cores":     details.Deployment.CPUCores,
			"memory_mb":     details.Deployment.MemoryMB,
			"started_at":    details.Deployment.StartedAt,
			"finished_at":   details.Deployment.FinishedAt,
		},
		"metrics": gin.H{
			"requests": gin.H{
				"total":      details.Metrics.RequestCountTotal,
				"last_hour":  details.Metrics.RequestCount1h,
				"last_24h":   details.Metrics.RequestCount24h,
				"per_second": details.Metrics.RequestsPerSecond,
			},
			"latency": gin.H{
				"p50_ms": details.Metrics.LatencyP50Ms,
				"p90_ms": details.Metrics.LatencyP90Ms,
				"p99_ms": details.Metrics.LatencyP99Ms,
			},
			"bandwidth": gin.H{
				"sent_bytes":     details.Metrics.BandwidthSentBytes,
				"received_bytes": details.Metrics.BandwidthRecvBytes,
			},
			"last_updated": details.Metrics.LastUpdated,
		},
		"pods":      details.Pods,
		"resources": details.Resources,
		"scaling":   details.Scaling,
	}

	c.JSON(http.StatusOK, response)
}

// GetDeploymentsList godoc
// @Summary      List all deployments with metrics
// @Description  Get a list of all deployments for the authenticated user with summary metrics
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  map[string]interface{}
// @Failure      401  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /deployments [get]
func (h *DeploymentDetailsHandler) GetDeploymentsList(c *gin.Context) {
	// Get user from context (set by auth middleware)
	user, userExists := c.Get("auth.user")

	var deployments []domain.DeploymentRecord

	if !userExists {
		// Fallback for when auth is disabled (dev mode)
		logs.Debugf("http", "list deployments requested_by=%s (no user context)", c.GetString("auth.sub"))
		deployments = h.deploymentService.ListDeployments()
	} else {
		actualUser := user.(domain.User)
		logs.Debugf("http", "list deployments user_id=%s", actualUser.UserID)
		deployments = h.deploymentService.ListDeploymentsByUser(actualUser.UserID)
	}

	// Get summaries with metrics
	summaries, err := h.detailsService.GetDeploymentSummaries(deployments)
	if err != nil {
		logs.Errorf("http", "failed to get deployment summaries: %v", err)
		// Fallback to basic deployment list
		c.JSON(http.StatusOK, gin.H{
			"deployments": deployments,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deployments": summaries,
	})
}
