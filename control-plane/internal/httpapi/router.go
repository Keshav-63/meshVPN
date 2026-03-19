package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"MeshVPN-slef-hosting/control-plane/internal/auth"
	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type DeployRequestPayload struct {
	Repo                 string            `json:"repo" binding:"required"`
	Port                 int               `json:"port"`
	Subdomain            string            `json:"subdomain"`
	ScalingMode          string            `json:"scaling_mode"`
	MinReplicas          int               `json:"min_replicas"`
	MaxReplicas          int               `json:"max_replicas"`
	CPUTargetUtilization int               `json:"cpu_target_utilization"`
	CPURequestMilli      int               `json:"cpu_request_milli"`
	CPULimitMilli        int               `json:"cpu_limit_milli"`
	NodeSelector         map[string]string `json:"node_selector"`
	Env                  map[string]string `json:"env"`
	BuildArgs            map[string]string `json:"build_args"`
	CPUCores             float64           `json:"cpu_cores"`
	MemoryMB             int               `json:"memory_mb"`
}

func NewRouter(cfg config.ControlPlaneConfig, deploymentService *service.DeploymentService) *gin.Engine {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		logs.Debugf("http", "health check")
		c.JSON(http.StatusOK, gin.H{
			"status": "LaptopCloud running",
		})
	})
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	protected := router.Group("/")
	protected.Use(auth.RequireSupabaseGitHub(auth.MiddlewareConfig{
		JWTSecret:   cfg.SupabaseJWTSecret,
		RequireAuth: cfg.RequireAuth,
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

		rec, err := deploymentService.EnqueueDeploy(c.Request.Context(), service.DeployRequest{
			Repo:         payload.Repo,
			Port:         payload.Port,
			Subdomain:    payload.Subdomain,
			ScalingMode:  payload.ScalingMode,
			MinReplicas:  payload.MinReplicas,
			MaxReplicas:  payload.MaxReplicas,
			CPUTarget:    payload.CPUTargetUtilization,
			CPURequest:   payload.CPURequestMilli,
			CPULimit:     payload.CPULimitMilli,
			NodeSelector: payload.NodeSelector,
			Env:          payload.Env,
			BuildArgs:    payload.BuildArgs,
			CPUCores:     payload.CPUCores,
			MemoryMB:     payload.MemoryMB,
			RequestedBy:  c.GetString("auth.sub"),
		})
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "repo is required") || strings.Contains(err.Error(), "invalid build_args") || strings.Contains(err.Error(), "invalid env var name") || strings.Contains(err.Error(), "invalid scaling_mode") || strings.Contains(err.Error(), "replicas") || strings.Contains(err.Error(), "cpu_") {
				status = http.StatusBadRequest
			}
			logs.Errorf("http", "enqueue failed err=%v", err)
			c.JSON(status, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"message":                "deployment queued",
			"deployment_id":          rec.DeploymentID,
			"status":                 rec.Status,
			"repo":                   rec.Repo,
			"subdomain":              rec.Subdomain,
			"port":                   rec.Port,
			"scaling_mode":           rec.ScalingMode,
			"min_replicas":           rec.MinReplicas,
			"max_replicas":           rec.MaxReplicas,
			"cpu_target_utilization": rec.CPUTarget,
			"cpu_request_milli":      rec.CPURequest,
			"cpu_limit_milli":        rec.CPULimit,
			"node_selector":          rec.NodeSelector,
			"cpu_cores":              rec.CPUCores,
			"memory_mb":              rec.MemoryMB,
		})
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

	return router
}
