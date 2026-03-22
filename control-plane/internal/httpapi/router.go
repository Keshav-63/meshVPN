package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"MeshVPN-slef-hosting/control-plane/internal/auth"
	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type DeployRequestPayload struct {
	Repo      string            `json:"repo" binding:"required"`
	Port      int               `json:"port"`
	Subdomain string            `json:"subdomain"` // Optional - auto-generated if empty
	Package   string            `json:"package"`   // small, medium, large
	Env       map[string]string `json:"env"`
	BuildArgs map[string]string `json:"build_args"`

	// Advanced options (optional - overridden by package if subscriber)
	ScalingMode          string            `json:"scaling_mode"`
	MinReplicas          int               `json:"min_replicas"`
	MaxReplicas          int               `json:"max_replicas"`
	CPUTargetUtilization int               `json:"cpu_target_utilization"`
	CPURequestMilli      int               `json:"cpu_request_milli"`
	CPULimitMilli        int               `json:"cpu_limit_milli"`
	NodeSelector         map[string]string `json:"node_selector"`
	CPUCores             float64           `json:"cpu_cores"`
	MemoryMB             int               `json:"memory_mb"`
}

func NewRouter(cfg config.ControlPlaneConfig, deploymentService *service.DeploymentService, userRepo auth.UserRepository, analyticsRepo AnalyticsRepository) *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		logs.Debugf("http", "health check")
		c.JSON(http.StatusOK, gin.H{
			"status": "LaptopCloud running",
		})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Create analytics handler
	var analyticsHandler *AnalyticsHandler
	if analyticsRepo != nil {
		analyticsHandler = NewAnalyticsHandler(deploymentService, analyticsRepo)
	}

	protected := router.Group("/")
	protected.Use(auth.RequireSupabaseGitHub(auth.MiddlewareConfig{
		JWTSecret:   cfg.SupabaseJWTSecret,
		RequireAuth: cfg.RequireAuth,
		UserRepo:    userRepo,
	}))

	protected.GET("/auth/whoami", func(c *gin.Context) {
		logs.Debugf("http", "whoami request sub=%s", c.GetString("auth.sub"))
		c.JSON(http.StatusOK, gin.H{
			"sub":      c.GetString("auth.sub"),
			"email":    c.GetString("auth.email"),
			"provider": c.GetString("auth.provider"),
		})
	})

	protected.POST("/deploy", func(c *gin.Context) {
		logs.Debugf("http", "deploy request received")
		var payload DeployRequestPayload
		if err := c.BindJSON(&payload); err != nil {
			logs.Errorf("http", "invalid deploy payload err=%v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "repo is required",
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
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid package '%s'. must be: small, medium, large", packageName),
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

		rec, err := deploymentService.EnqueueDeploy(c.Request.Context(), service.DeployRequest{
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
			c.JSON(status, gin.H{
				"error": err.Error(),
			})
			return
		}

		response := gin.H{
			"message":                "deployment queued",
			"deployment_id":          rec.DeploymentID,
			"status":                 rec.Status,
			"repo":                   rec.Repo,
			"subdomain":              rec.Subdomain,
			"url":                    fmt.Sprintf("https://%s.%s", rec.Subdomain, "keshavstack.tech"),
			"port":                   rec.Port,
			"package":                packageName,
			"cpu_cores":              packageSpec.CPUCores,
			"memory_mb":              packageSpec.MemoryMB,
			"scaling_mode":           rec.ScalingMode,
			"min_replicas":           rec.MinReplicas,
			"max_replicas":           rec.MaxReplicas,
			"cpu_target_utilization": rec.CPUTarget,
			"autoscaling_enabled":    isSubscriber,
		}

		c.JSON(http.StatusAccepted, response)
	})

	protected.GET("/deployments", func(c *gin.Context) {
		logs.Debugf("http", "list deployments requested_by=%s", c.GetString("auth.sub"))
		c.JSON(http.StatusOK, gin.H{
			"deployments": deploymentService.ListDeployments(),
		})
	})

	protected.GET("/deployments/:id/build-logs", func(c *gin.Context) {
		logs.Debugf("http", "build logs request deployment_id=%s", c.Param("id"))
		rec, err := deploymentService.GetDeployment(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_id": rec.DeploymentID,
			"status":        rec.Status,
			"build_logs":    rec.BuildLogs,
		})
	})

	protected.GET("/deployments/:id/app-logs", func(c *gin.Context) {
		logs.Debugf("http", "app logs request deployment_id=%s", c.Param("id"))
		tail := 200
		if raw := strings.TrimSpace(c.Query("tail")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "tail must be a positive integer"})
				return
			}
			if parsed > 5000 {
				parsed = 5000
			}
			tail = parsed
		}

		rec, appLogs, err := deploymentService.GetAppLogs(c.Param("id"), tail)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			if strings.Contains(err.Error(), "no running container") || strings.Contains(err.Error(), "no running") {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"error":            fmt.Sprintf("%v", err),
				"container":        rec.Container,
				"application_logs": appLogs,
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"deployment_id":    rec.DeploymentID,
			"container":        rec.Container,
			"tail":             tail,
			"application_logs": appLogs,
		})
	})

	// Analytics endpoints (if analytics repository is available)
	if analyticsHandler != nil {
		protected.GET("/deployments/:id/analytics", analyticsHandler.GetAnalytics)
		protected.GET("/deployments/:id/analytics/stream", analyticsHandler.StreamAnalytics)
		logs.Infof("http", "analytics endpoints registered")
	}

	return router
}
