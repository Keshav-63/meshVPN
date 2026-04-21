package store

import (
	"database/sql"
	"fmt"
	"time"

	"MeshVPN-slef-hosting/control-plane/internal/config"
	"MeshVPN-slef-hosting/control-plane/internal/logs"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Dependencies struct {
	DeploymentRepo DeploymentRepository
	JobRepo        JobRepository
	UserRepo       *PostgresUserRepository
	AnalyticsRepo  *PostgresAnalyticsRepository
	WorkerRepo     WorkerRepository
	DB             *sql.DB
	HasDatabase    bool
}

func Initialize(cfg config.ControlPlaneConfig) (Dependencies, func(), error) {
	if cfg.DatabaseURL == "" {
		logs.Infof("store", "DATABASE_URL not set, using in-memory repositories (deployments/jobs only)")
		return Dependencies{
			DeploymentRepo: NewInMemoryDeploymentRepository(),
			JobRepo:        NewInMemoryJobRepository(),
			HasDatabase:    false,
		}, nil, nil
	}

	logs.Infof("store", "initializing postgres repositories")
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return Dependencies{}, nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(15 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return Dependencies{}, nil, fmt.Errorf("ping postgres connection: %w", err)
	}

	logs.Infof("store", "postgres connection established max_open=%d max_idle=%d max_lifetime=%s",
		15, 5, 15*time.Minute)

	repo := NewPostgresDeploymentRepository(db)
	if err := EnsureMigrations(db); err != nil {
		_ = db.Close()
		return Dependencies{}, nil, fmt.Errorf("ensure migrations: %w", err)
	}
	jobRepo := NewPostgresJobRepository(db)
	userRepo := NewPostgresUserRepository(db)
	analyticsRepo := NewPostgresAnalyticsRepository(db)
	workerRepo := NewPostgresWorkerRepository(db)

	cleanup := func() {
		logs.Infof("store", "closing postgres connection")
		_ = db.Close()
	}

	logs.Infof("store", "postgres repositories ready (deployments/jobs/users/analytics/workers)")

	return Dependencies{
		DeploymentRepo: repo,
		JobRepo:        jobRepo,
		UserRepo:       userRepo,
		AnalyticsRepo:  analyticsRepo,
		WorkerRepo:     workerRepo,
		DB:             db,
		HasDatabase:    true,
	}, cleanup, nil
}
