package httpapi

import (
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/auth"
	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/service"
	"MeshVPN-slef-hosting/control-plane/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// DeployRequestPayload represents the deployment request body
type DeployRequestPayload struct {
	Repo      string            `json:"repo" binding:"required" example:"https://github.com/user/repo"`
	Port      int               `json:"port" example:"3000"`
	Subdomain string            `json:"subdomain" example:"myapp"` // Optional - auto-generated if empty
	Package   string            `json:"package" example:"small"`   // small, medium, large
	Env       map[string]string `json:"env"`
	BuildArgs map[string]string `json:"build_args"`

	// Advanced options (optional - overridden by package if subscriber)
	ScalingMode          string            `json:"scaling_mode" example:"horizontal"`
	MinReplicas          int               `json:"min_replicas" example:"1"`
	MaxReplicas          int               `json:"max_replicas" example:"3"`
	CPUTargetUtilization int               `json:"cpu_target_utilization" example:"70"`
	CPURequestMilli      int               `json:"cpu_request_milli" example:"500"`
	CPULimitMilli        int               `json:"cpu_limit_milli" example:"1000"`
	NodeSelector         map[string]string `json:"node_selector"`
	CPUCores             float64           `json:"cpu_cores" example:"0.5"`
	MemoryMB             int               `json:"memory_mb" example:"512"`
}

func NewRouter(cfg config.ControlPlaneConfig, deploymentService *service.DeploymentService, userRepo auth.UserRepository, analyticsRepo AnalyticsRepository, workerRepo store.WorkerRepository, jobRepo store.JobRepository, deploymentRepo store.DeploymentRepository) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())

	// Custom logger that skips /metrics endpoint
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path != "/metrics" {
			gin.Logger()(c)
		} else {
			c.Next()
		}
	})

	// CORS configuration for Next.js frontend
	corsConfig := cors.Config{
		AllowOrigins:     []string{cfg.FrontendURL},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))
	logs.Infof("http", "CORS enabled for origin: %s", cfg.FrontendURL)

	// Initialize handlers
	handlers := NewHandlers(deploymentService)

	router.GET("/health", handlers.HealthCheck)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Telemetry endpoints (public - no auth required, called by Traefik/proxies)
	if analyticsRepo != nil {
		telemetryHandler := NewTelemetryHandler(analyticsRepo)
		router.POST("/api/telemetry/deployment-request", telemetryHandler.RecordDeploymentRequest)
		router.POST("/api/telemetry/deployment-request/batch", telemetryHandler.RecordDeploymentRequestBatch)
		logs.Infof("http", "telemetry endpoints registered")
	}

	// Create analytics handler
	var analyticsHandler *AnalyticsHandler
	if analyticsRepo != nil {
		analyticsHandler = NewAnalyticsHandler(deploymentService, analyticsRepo)
	}

	protected := router.Group("/")
	protected.Use(auth.RequireSupabaseAuth(auth.MiddlewareConfig{
		SupabaseURL:     cfg.SupabaseURL,
		SupabaseAnonKey: cfg.SupabaseAnonKey,
		JWTSecret:       cfg.SupabaseJWTSecret,
		RequireAuth:     cfg.RequireAuth,
		UserRepo:        userRepo,
	}))

	protected.GET("/auth/whoami", handlers.WhoAmI)
	protected.POST("/deploy", handlers.Deploy)
	protected.GET("/deployments", handlers.ListDeployments)
	protected.GET("/deployments/:id/build-logs", handlers.GetBuildLogs)
	protected.GET("/deployments/:id/app-logs", handlers.GetAppLogs)

	// Analytics endpoints (if analytics repository is available)
	if analyticsHandler != nil {
		protected.GET("/deployments/:id/analytics", analyticsHandler.GetAnalytics)
		protected.GET("/deployments/:id/analytics/stream", analyticsHandler.StreamAnalytics)
		logs.Infof("http", "analytics endpoints registered")
	}

	// Platform-level analytics endpoints
	if workerRepo != nil && deploymentRepo != nil && analyticsRepo != nil {
		platformAnalyticsHandler := NewPlatformAnalyticsHandler(
			deploymentRepo,
			workerRepo,
			jobRepo,
			analyticsRepo,
		)

		protected.GET("/platform/analytics", platformAnalyticsHandler.GetPlatformAnalytics)
		protected.GET("/platform/workers/:id/analytics", platformAnalyticsHandler.GetWorkerAnalytics)
		logs.Infof("http", "platform analytics endpoints registered")
	}

	// Worker API endpoints (no user auth - workers use internal routes)
	if workerRepo != nil && jobRepo != nil {
		workerHandler := NewWorkerHandler(workerRepo, jobRepo)

		workerAPI := router.Group("/api/workers")
		// TODO: Add worker authentication middleware (shared secret or mTLS)
		workerAPI.POST("/register", workerHandler.Register)
		workerAPI.POST("/:id/heartbeat", workerHandler.Heartbeat)
		workerAPI.GET("/:id/claim-job", workerHandler.ClaimJob)
		workerAPI.POST("/:id/job-complete", workerHandler.JobComplete)
		workerAPI.POST("/:id/job-failed", workerHandler.JobFailed)

		// Admin endpoints (require user auth)
		protected.GET("/workers", workerHandler.List)

		logs.Infof("http", "worker API endpoints registered")
	}

	// Swagger documentation endpoint
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}
