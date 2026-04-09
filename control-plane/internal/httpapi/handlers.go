package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	deploymentService *service.DeploymentService
	enableCPUHPA      bool
}

func NewHandlers(deploymentService *service.DeploymentService, enableCPUHPA bool) *Handlers {
	return &Handlers{
		deploymentService: deploymentService,
		enableCPUHPA:      enableCPUHPA,
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
	var actualUser domain.User
	if user, userExists := c.Get("auth.user"); userExists {
		actualUser = user.(domain.User)
	}
	requestedBy := strings.TrimSpace(c.GetString("auth.sub"))
	userID := strings.TrimSpace(actualUser.UserID)
	if userID == "" {
		userID = requestedBy
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

	// Autoscaling is enabled for all users.
	scalingMode := "horizontal"
	minReplicas := 1
	maxReplicas := packageSpec.MaxReplicas
	cpuTarget := 70 // Default CPU target percentage
	memoryTarget := getMemoryTargetUtilization()

	if payload.CPUTargetUtilization > 0 && payload.CPUTargetUtilization <= 100 {
		cpuTarget = payload.CPUTargetUtilization
	}
	if payload.MinReplicas > 0 {
		minReplicas = payload.MinReplicas
	}
	if payload.MaxReplicas > 0 && payload.MaxReplicas <= packageSpec.MaxReplicas {
		maxReplicas = payload.MaxReplicas
	}

	logs.Infof("http", "deploy package=%s scaling=%s user_id=%s requested_by=%s", packageName, scalingMode, userID, requestedBy)

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
		CPULimit:     int(packageSpec.CPUCores * 2000), // Keep limit proportional to package size
		NodeSelector: payload.NodeSelector,
		Env:          payload.Env,
		BuildArgs:    payload.BuildArgs,
		RequestedBy:  requestedBy,
		UserID:       userID,
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
		Message:                 "deployment queued",
		DeploymentID:            rec.DeploymentID,
		Status:                  rec.Status,
		Repo:                    rec.Repo,
		Subdomain:               rec.Subdomain,
		URL:                     buildDeploymentURL(rec.Subdomain),
		Port:                    rec.Port,
		Package:                 packageName,
		CPUCores:                packageSpec.CPUCores,
		MemoryMB:                packageSpec.MemoryMB,
		ScalingMode:             rec.ScalingMode,
		MinReplicas:             rec.MinReplicas,
		MaxReplicas:             rec.MaxReplicas,
		CPUTargetUtilization:    rec.CPUTarget,
		MemoryTargetUtilization: memoryTarget,
		AutoscalingEnabled:      rec.ScalingMode == "horizontal" && h.enableCPUHPA,
	}

	c.JSON(http.StatusAccepted, response)
}

func buildDeploymentURL(subdomain string) string {
	baseDomain := strings.Trim(strings.ToLower(strings.TrimSpace(os.Getenv("APP_BASE_DOMAIN"))), ".")
	if baseDomain == "" {
		baseDomain = "keshavstack.tech"
	}

	return fmt.Sprintf("https://%s.%s", subdomain, baseDomain)
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
	authSub := strings.TrimSpace(c.GetString("auth.sub"))

	// Get user from context (set by auth middleware)
	user, userExists := c.Get("auth.user")

	if !userExists {
		if authSub != "" {
			logs.Debugf("http", "list deployments user_id=%s (fallback from auth.sub)", authSub)
			c.JSON(http.StatusOK, gin.H{
				"deployments": h.deploymentService.ListDeploymentsByUser(authSub),
			})
			return
		}

		// Fallback for when auth is disabled (dev mode)
		logs.Debugf("http", "list deployments requested_by=%s (no user context)", c.GetString("auth.sub"))
		c.JSON(http.StatusOK, gin.H{
			"deployments": h.deploymentService.ListDeployments(),
		})
		return
	}

	actualUser := user.(domain.User)
	logs.Debugf("http", "list deployments user_id=%s", actualUser.UserID)

	// Return only deployments owned by this user
	c.JSON(http.StatusOK, gin.H{
		"deployments": h.deploymentService.ListDeploymentsByUser(actualUser.UserID),
	})
}

func getMemoryTargetUtilization() int {
	raw := strings.TrimSpace(os.Getenv("HPA_MEMORY_TARGET_UTILIZATION"))
	if raw == "" {
		return 75
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > 100 {
		return 75
	}

	return parsed
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
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /deployments/{id}/build-logs [get]
func (h *Handlers) GetBuildLogs(c *gin.Context) {
	deploymentID := c.Param("id")
	logs.Debugf("http", "build logs request deployment_id=%s", deploymentID)

	rec, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	// Check user authorization
	user, userExists := c.Get("auth.user")
	if userExists {
		actualUser := user.(domain.User)
		if rec.UserID != "" && rec.UserID != actualUser.UserID {
			logs.Errorf("http", "user %s attempted to access deployment %s owned by %s",
				actualUser.UserID, deploymentID, rec.UserID)
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "access denied"})
			return
		}
	}

	c.JSON(http.StatusOK, BuildLogsResponse{
		DeploymentID: rec.DeploymentID,
		Status:       rec.Status,
		BuildLogs:    rec.BuildLogs,
	})
}

// StreamBuildLogs provides real-time build logs via Server-Sent Events (GET /deployments/:id/build-logs/stream)
// @Summary      Stream live build logs
// @Description  Stream incremental build logs while deployment is building
// @Tags         Deployments
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id     path      string  true   "Deployment ID"
// @Param        token  query     string  false  "JWT token (SSE doesn't support headers)"
// @Success      200    {string}  string  "SSE stream"
// @Failure      401    {object}  ErrorResponse
// @Failure      403    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Router       /deployments/{id}/build-logs/stream [get]
func (h *Handlers) StreamBuildLogs(c *gin.Context) {
	deploymentID := c.Param("id")
	logs.Debugf("http", "build logs SSE request deployment_id=%s", deploymentID)

	rec, err := h.deploymentService.GetDeployment(deploymentID)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}

	// Check user authorization
	if user, userExists := c.Get("auth.user"); userExists {
		actualUser := user.(domain.User)
		if rec.UserID != "" && rec.UserID != actualUser.UserID {
			logs.Errorf("http", "user %s attempted to stream build logs for deployment %s owned by %s",
				actualUser.UserID, deploymentID, rec.UserID)
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "access denied"})
			return
		}
	}

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	lastSentOffset := 0

	sendDelta := func(current domain.DeploymentRecord) bool {
		if len(current.BuildLogs) > lastSentOffset {
			chunk := current.BuildLogs[lastSentOffset:]
			lastSentOffset = len(current.BuildLogs)
			h.sendBuildLogSSE(c, deploymentID, current.Status, chunk, lastSentOffset, false)
		}

		if isTerminalBuildStatus(current.Status) {
			h.sendBuildLogSSE(c, deploymentID, current.Status, "", lastSentOffset, true)
			return true
		}

		return false
	}

	if sendDelta(rec) {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			logs.Infof("http", "build logs SSE closed deployment_id=%s", deploymentID)
			return
		case <-ticker.C:
			current, getErr := h.deploymentService.GetDeployment(deploymentID)
			if getErr != nil {
				fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\":\"deployment not found\"}\n\n")
				c.Writer.Flush()
				return
			}

			if sendDelta(current) {
				return
			}
		}
	}
}

func (h *Handlers) sendBuildLogSSE(c *gin.Context, deploymentID string, status string, chunk string, offset int, complete bool) {
	payload := gin.H{
		"deployment_id": deploymentID,
		"status":        status,
		"offset":        offset,
		"chunk":         chunk,
		"complete":      complete,
		"timestamp":     time.Now().Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logs.Errorf("http", "failed to marshal build logs SSE payload deployment_id=%s err=%v", deploymentID, err)
		return
	}

	if complete {
		fmt.Fprintf(c.Writer, "event: complete\ndata: %s\n\n", string(data))
	} else {
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	}
	c.Writer.Flush()
}

func isTerminalBuildStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "failed":
		return true
	default:
		return false
	}
}

// GetAppLogs godoc
// @Summary      Get application runtime logs
// @Description  Retrieve runtime logs for a specific deployment
// @Tags         Deployments
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      string  true   "Deployment ID"
// @Param        tail  query     int     false  "Number of log lines to retrieve (max 5000)" default(200)
// @Param        cursor query    int     false  "Byte cursor for incremental logs (returns delta when provided)"
// @Success      200   {object}  AppLogsResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Failure      500   {object}  ErrorResponse
// @Router       /deployments/{id}/app-logs [get]
func (h *Handlers) GetAppLogs(c *gin.Context) {
	deploymentID := c.Param("id")
	logs.Debugf("http", "app logs request deployment_id=%s", deploymentID)

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

	cursor := 0
	delta := false
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cursor must be a non-negative integer"})
			return
		}
		cursor = parsed
		delta = true
	}

	rec, appLogs, err := h.deploymentService.GetAppLogs(deploymentID, tail)
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

	// Check user authorization
	user, userExists := c.Get("auth.user")
	if userExists {
		actualUser := user.(domain.User)
		if rec.UserID != "" && rec.UserID != actualUser.UserID {
			logs.Errorf("http", "user %s attempted to access deployment %s owned by %s",
				actualUser.UserID, deploymentID, rec.UserID)
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "access denied"})
			return
		}
	}

	fullLength := len(appLogs)
	logsOut := appLogs
	if delta {
		if cursor > fullLength {
			cursor = 0
		}
		if cursor <= fullLength {
			logsOut = appLogs[cursor:]
		}
	}

	c.JSON(http.StatusOK, AppLogsResponse{
		DeploymentID:    rec.DeploymentID,
		Container:       rec.Container,
		Tail:            tail,
		Cursor:          cursor,
		NextCursor:      fullLength,
		Delta:           delta,
		ApplicationLogs: logsOut,
	})
}

// StreamAppLogs provides real-time application logs via Server-Sent Events (GET /deployments/:id/app-logs/stream)
// @Summary      Stream live application logs
// @Description  Stream incremental application log updates to avoid duplicate polling payloads
// @Tags         Deployments
// @Accept       json
// @Produce      text/event-stream
// @Security     BearerAuth
// @Param        id     path      string  true   "Deployment ID"
// @Param        tail   query     int     false  "Number of log lines to retrieve (max 5000)" default(200)
// @Param        token  query     string  false  "JWT token (SSE doesn't support headers)"
// @Success      200    {string}  string  "SSE stream"
// @Failure      400    {object}  ErrorResponse
// @Failure      401    {object}  ErrorResponse
// @Failure      403    {object}  ErrorResponse
// @Failure      404    {object}  ErrorResponse
// @Failure      500    {object}  ErrorResponse
// @Router       /deployments/{id}/app-logs/stream [get]
func (h *Handlers) StreamAppLogs(c *gin.Context) {
	deploymentID := c.Param("id")
	logs.Debugf("http", "app logs SSE request deployment_id=%s", deploymentID)

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

	rec, appLogs, err := h.deploymentService.GetAppLogs(deploymentID, tail)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		if strings.Contains(err.Error(), "no running container") || strings.Contains(err.Error(), "no running") {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("%v", err)})
		return
	}

	if user, userExists := c.Get("auth.user"); userExists {
		actualUser := user.(domain.User)
		if rec.UserID != "" && rec.UserID != actualUser.UserID {
			logs.Errorf("http", "user %s attempted to stream app logs for deployment %s owned by %s",
				actualUser.UserID, deploymentID, rec.UserID)
			c.JSON(http.StatusForbidden, ErrorResponse{Error: "access denied"})
			return
		}
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")

	lastCursor := 0
	if len(appLogs) > 0 {
		lastCursor = len(appLogs)
		h.sendAppLogSSE(c, deploymentID, rec.Container, appLogs, lastCursor, false, false)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			logs.Infof("http", "app logs SSE closed deployment_id=%s", deploymentID)
			return
		case <-ticker.C:
			latestRec, latestLogs, latestErr := h.deploymentService.GetAppLogs(deploymentID, tail)
			if latestErr != nil {
				fmt.Fprintf(c.Writer, "event: error\ndata: {\"error\":\"%s\"}\n\n", strings.ReplaceAll(latestErr.Error(), "\"", "'"))
				c.Writer.Flush()
				continue
			}

			currentLen := len(latestLogs)
			if currentLen < lastCursor {
				lastCursor = 0
				h.sendAppLogSSE(c, deploymentID, latestRec.Container, latestLogs, currentLen, false, true)
				lastCursor = currentLen
				continue
			}

			if currentLen > lastCursor {
				chunk := latestLogs[lastCursor:]
				lastCursor = currentLen
				h.sendAppLogSSE(c, deploymentID, latestRec.Container, chunk, lastCursor, false, false)
			}
		}
	}
}

func (h *Handlers) sendAppLogSSE(c *gin.Context, deploymentID string, container string, chunk string, cursor int, complete bool, reset bool) {
	payload := gin.H{
		"deployment_id":     deploymentID,
		"container":         container,
		"chunk":             chunk,
		"next_cursor":       cursor,
		"complete":          complete,
		"reset_full_buffer": reset,
		"timestamp":         time.Now().Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		logs.Errorf("http", "failed to marshal app logs SSE payload deployment_id=%s err=%v", deploymentID, err)
		return
	}

	if complete {
		fmt.Fprintf(c.Writer, "event: complete\ndata: %s\n\n", string(data))
	} else {
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	}
	c.Writer.Flush()
}
