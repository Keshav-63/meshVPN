package main

import (
	"context"
	"log"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/analytics"
	"MeshVPN-slef-hosting/control-plane/internal/auth"
	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/httpapi"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
	"MeshVPN-slef-hosting/control-plane/internal/runtime"
	"MeshVPN-slef-hosting/control-plane/internal/service"
	"MeshVPN-slef-hosting/control-plane/internal/store"
	"MeshVPN-slef-hosting/control-plane/internal/telemetry"

	_ "MeshVPN-slef-hosting/control-plane/docs" // Import generated swagger docs
)

// @title           MeshVPN Control Plane API
// @version         1.0
// @description     API for deploying and managing applications on MeshVPN platform
// @description     This API provides endpoints for deploying applications, managing deployments, viewing logs, and monitoring analytics.

// @contact.name   API Support
// @contact.url    https://github.com/keshavstack/MeshVPN-slef-hosting

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host      localhost:8080
// @BasePath  /

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter your JWT token in the format: Bearer {token}

func main() {
	cfg := config.Load()
	deps, cleanup, err := store.Initialize(cfg)
	if err != nil {
		log.Printf("deployment repository init failed, falling back to in-memory store: %v", err)
		deps = store.Dependencies{
			DeploymentRepo: store.NewInMemoryDeploymentRepository(),
			JobRepo:        store.NewInMemoryJobRepository(),
			HasDatabase:    false,
		}
	}
	if cleanup != nil {
		defer cleanup()
	}

	telemetry.Register()

	driver := runtime.NewDriverFromBackend(cfg.RuntimeBackend, cfg.K8sNamespace)
	runner := runtime.NewRunnerWithDriver(driver)
	deploymentService := service.NewDeploymentService(deps.DeploymentRepo, deps.JobRepo, runner)

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start either multi-worker distributor or single embedded worker
	if cfg.EnableMultiWorker && deps.WorkerRepo != nil {
		// Multi-worker mode: start job distributor (handles control-plane worker registration)
		distributor := service.NewJobDistributor(deps.JobRepo, deps.WorkerRepo, deps.DeploymentRepo, cfg)
		go distributor.Start(workerCtx)
		logs.Infof("main", "multi-worker mode enabled strategy=%s control_plane_as_worker=%t",
			cfg.JobPlacementStrategy, cfg.ControlPlaneAsWorker)

		// If control-plane acts as worker, start embedded worker to execute jobs
		if cfg.ControlPlaneAsWorker {
			worker := service.NewDeploymentWorker(deps.DeploymentRepo, deps.JobRepo, runner, cfg.WorkerPollInterval, cfg.EnableCPUHPA, cfg.ControlPlaneWorkerID)
			go worker.Start(workerCtx)
			logs.Infof("main", "embedded worker started for control-plane (worker_id=%s)", cfg.ControlPlaneWorkerID)
		}
	} else {
		// Single-worker mode: start embedded worker
		worker := service.NewDeploymentWorker(deps.DeploymentRepo, deps.JobRepo, runner, cfg.WorkerPollInterval, cfg.EnableCPUHPA, cfg.ControlPlaneWorkerID)
		go worker.Start(workerCtx)
		logs.Infof("main", "single-worker mode: embedded worker started")
	}

	// Start analytics collector if database is available
	if deps.HasDatabase && deps.AnalyticsRepo != nil {
		collector := analytics.NewMetricsCollector(deps.AnalyticsRepo, deps.WorkerRepo, deps.DeploymentRepo, cfg.K8sNamespace, "kubectl")
		go collector.Start(workerCtx, 1*time.Minute)
		logs.Infof("main", "analytics collector started interval=1m")
	}

	// Prepare user repository for auth middleware
	var userRepo auth.UserRepository
	if deps.UserRepo != nil {
		userRepo = deps.UserRepo
	}

	// Prepare analytics repository for analytics endpoints
	var analyticsRepo httpapi.AnalyticsRepository
	if deps.AnalyticsRepo != nil {
		analyticsRepo = deps.AnalyticsRepo
	}

	// Initialize Kubernetes client and deployment details service
	var detailsService *service.DeploymentDetailsService
	if deps.HasDatabase && deps.AnalyticsRepo != nil {
		k8sClient := analytics.NewKubernetesClient(cfg.K8sNamespace, "kubectl")
		detailsService = service.NewDeploymentDetailsService(deps.DeploymentRepo, deps.AnalyticsRepo, k8sClient)
		logs.Infof("main", "deployment details service initialized with k8s client")
	}

	logs.Infof("main", "starting router require_auth=%t has_database=%t analytics=%t details_service=%t",
		cfg.RequireAuth, deps.HasDatabase, analyticsRepo != nil, detailsService != nil)
	router := httpapi.NewRouter(cfg, deploymentService, detailsService, userRepo, analyticsRepo, deps.WorkerRepo, deps.JobRepo, deps.DeploymentRepo)

	if err := router.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
