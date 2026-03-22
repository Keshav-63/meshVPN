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
)

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
	worker := service.NewDeploymentWorker(deps.DeploymentRepo, deps.JobRepo, runner, cfg.WorkerPollInterval, cfg.EnableCPUHPA)

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.Start(workerCtx)

	// Start analytics collector if database is available
	if deps.HasDatabase && deps.AnalyticsRepo != nil {
		collector := analytics.NewMetricsCollector(deps.AnalyticsRepo, cfg.K8sNamespace, "kubectl")
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

	logs.Infof("main", "starting router require_auth=%t has_database=%t analytics=%t", cfg.RequireAuth, deps.HasDatabase, analyticsRepo != nil)
	router := httpapi.NewRouter(cfg, deploymentService, userRepo, analyticsRepo)

	if err := router.Run("0.0.0.0:8080"); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
