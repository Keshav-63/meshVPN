package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	deploymentService *service.DeploymentService
}

func NewHandlers(deploymentService *service.DeploymentService) *Handlers {
	return &Handlers{
		deploymentService: deploymentService,
	}
}

// HealthCheck godoc
// @Summary      Health check
// @Description  Check if the API is running
// @Tags         System
// @Produce      json
// @Success      200  {object}  HealthResponse
// @Router       /health [get]
func (h *Handlers) HealthCheck(c *gin.Context) {
	logs.Debugf("http", "health check")
	c.JSON(http.StatusOK, HealthResponse{
		Status: "LaptopCloud running",
	})
}

// WhoAmI godoc
// @Summary      Get current user information
// @Description  Retrieve authenticated user's information
// @Tags         Authentication
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  WhoAmIResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /auth/whoami [get]
func (h *Handlers) WhoAmI(c *gin.Context) {
	logs.Debugf("http", "whoami request sub=%s", c.GetString("auth.sub"))
	c.JSON(http.StatusOK, WhoAmIResponse{
		Sub:      c.GetString("auth.sub"),
		Email:    c.GetString("auth.email"),
		Provider: c.GetString("auth.provider"),
	})
}

// Deploy godoc
// @Summary      Deploy a new application
// @Description  Queue a new application deployment from a git repository
// @Tags         Deployments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      DeployRequestPayload  true  "Deployment configuration"
// @Success      202      {object}  DeployResponse
// @Failure      400      {object}  ErrorResponse
// @Failure      401      {object}  ErrorResponse
// @Failure      500      {object}  ErrorResponse
// @Router       /deploy [post]
func (h *Handlers) Deploy(c *gin.Context) {
	logs.Debugf("http", "deploy request received")
	var payload DeployRequestPayload
	if err := c.BindJSON(&payload); err != nil {
		logs.Errorf("http", "invalid deploy payload err=%v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "repo is required",
		})
		return
	}

	// Get user from context (set by auth middleware)
	user, userExists := c.Get("auth.user")
	var actualUser domain.User
	var isSubscriber bool

	if userExists {
		actualUser = user.(domain.User)
		isSubscriber = actualUser.IsSubscriber
	} else {
		// Fallback for when auth is disabled
		isSubscriber = false
	}

	// Validate and get package specification
	packageName := strings.ToLower(strings.TrimSpace(payload.Package))
	if packageName == "" {
		packageName = "small" // Default package
	}

	if !domain.IsValidPackage(packageName) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("invalid package '%s'. must be: small, medium, large", packageName),
		})
		return
	}

	packageSpec, _ := domain.GetPackageSpec(domain.ResourcePackage(packageName))

	// Determine scaling behavior based on subscription status
	scalingMode := "none"
	minReplicas := 1
	maxReplicas := 1
	cpuTarget := 70 // Default CPU target percentage

	if isSubscriber {
		// Subscribers get autoscaling enabled
		scalingMode = "horizontal"
		minReplicas = 1
		maxReplicas = packageSpec.MaxReplicas

		// Allow subscribers to customize scaling parameters
		if payload.CPUTargetUtilization > 0 && payload.CPUTargetUtilization <= 100 {
			cpuTarget = payload.CPUTargetUtilization
		}
		if payload.MinReplicas > 0 {
			minReplicas = payload.MinReplicas
		}
		if payload.MaxReplicas > 0 && payload.MaxReplicas <= packageSpec.MaxReplicas {
			maxReplicas = payload.MaxReplicas
		}
	}

	logs.Infof("http", "deploy package=%s subscriber=%t scaling=%s user_id=%s", packageName, isSubscriber, scalingMode, actualUser.UserID)

	rec, err := h.deploymentService.EnqueueDeploy(c.Request.Context(), service.DeployRequest{
		Repo:         payload.Repo,
		Port:         payload.Port,
		Subdomain:    payload.Subdomain,
		Package:      packageName,
		CPUCores:     packageSpec.CPUCores,
		MemoryMB:     packageSpec.MemoryMB,
		ScalingMode:  scalingMode,
		MinReplicas:  minReplicas,
		MaxReplicas:  maxReplicas,
		CPUTarget:    cpuTarget,
		CPURequest:   int(packageSpec.CPUCores * 1000), // Convert to millicores
		CPULimit:     500,                              // Safety limit
		NodeSelector: payload.NodeSelector,
		Env:          payload.Env,
		BuildArgs:    payload.BuildArgs,
		RequestedBy:  c.GetString("auth.sub"),
		UserID:       actualUser.UserID,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "repo is required") ||
			strings.Contains(err.Error(), "invalid") ||
			strings.Contains(err.Error(), "subdomain") ||
			strings.Contains(err.Error(), "already in use") {
			status = http.StatusBadRequest
		}
		logs.Errorf("http", "enqueue failed err=%v", err)
		c.JSON(status, ErrorResponse{
			Error: err.Error(),
		})
		return
	}

	response := DeployResponse{
		Message:              "deployment queued",
		DeploymentID:         rec.DeploymentID,
		Status:               rec.Status,
		Repo:                 rec.Repo,
		Subdomain:            rec.Subdomain,
		URL:                  fmt.Sprintf("https://%s.%s", rec.Subdomain, "keshavstack.tech"),
		Port:                 rec.Port,
		Package:              packageName,
		CPUCores:             packageSpec.CPUCores,
		MemoryMB:             packageSpec.MemoryMB,
		ScalingMode:          rec.ScalingMode,
		MinReplicas:          rec.MinReplicas,
		MaxReplicas:          rec.MaxReplicas,
		CPUTargetUtilization: rec.CPUTarget,
		AutoscalingEnabled:   isSubscriber,
	}

	c.JSON(http.StatusAccepted, response)
}

// ListDeployments godoc
// @Summary      List all deployments
// @Description  Get a list of all deployments for the authenticated user
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  DeploymentListResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /deployments [get]
func (h *Handlers) ListDeployments(c *gin.Context) {
	logs.Debugf("http", "list deployments requested_by=%s", c.GetString("auth.sub"))
	c.JSON(http.StatusOK, gin.H{
		"deployments": h.deploymentService.ListDeployments(),
	})
}

// GetBuildLogs godoc
// @Summary      Get deployment build logs
// @Description  Retrieve build logs for a specific deployment
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "Deployment ID"
// @Success      200  {object}  BuildLogsResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /deployments/{id}/build-logs [get]
func (h *Handlers) GetBuildLogs(c *gin.Context) {
	logs.Debugf("http", "build logs request deployment_id=%s", c.Param("id"))
	rec, err := h.deploymentService.GetDeployment(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, BuildLogsResponse{
		DeploymentID: rec.DeploymentID,
		Status:       rec.Status,
		BuildLogs:    rec.BuildLogs,
	})
}

// GetAppLogs godoc
// @Summary      Get application runtime logs
// @Description  Retrieve runtime logs for a specific deployment
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string  true   "Deployment ID"
// @Param        tail  query     int     false  "Number of log lines to retrieve (max 5000)" default(200)
// @Success      200   {object}  AppLogsResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Failure      500   {object}  ErrorResponse
// @Router       /deployments/{id}/app-logs [get]
func (h *Handlers) GetAppLogs(c *gin.Context) {
	logs.Debugf("http", "app logs request deployment_id=%s", c.Param("id"))
	tail := 200
	if raw := strings.TrimSpace(c.Query("tail")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "tail must be a positive integer"})
			return
		}
		if parsed > 5000 {
			parsed = 5000
		}
		tail = parsed
	}

	rec, appLogs, err := h.deploymentService.GetAppLogs(c.Param("id"), tail)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		if strings.Contains(err.Error(), "no running container") || strings.Contains(err.Error(), "no running") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":            fmt.Sprintf("%v", err),
			"container":        rec.Container,
			"application_logs": appLogs,
		})
		return
	}

	c.JSON(http.StatusOK, AppLogsResponse{
		DeploymentID:    rec.DeploymentID,
		Container:       rec.Container,
		Tail:            tail,
		ApplicationLogs: appLogs,
	})
}
