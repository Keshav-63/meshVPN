package main

import (
	"context"
	"log"

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

	logs.Infof("main", "starting router require_auth=%t has_database=%t", cfg.RequireAuth, deps.HasDatabase)
	router := httpapi.NewRouter(cfg, deploymentService)

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}
