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
	DB             *sql.DB
	HasDatabase    bool
}

func Initialize(cfg config.ControlPlaneConfig) (Dependencies, func(), error) {
	if cfg.DatabaseURL == "" {
		logs.Infof("store", "DATABASE_URL not set, using in-memory repositories")
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

	repo := NewPostgresDeploymentRepository(db)
	if err := EnsureMigrations(db); err != nil {
		_ = db.Close()
		return Dependencies{}, nil, fmt.Errorf("ensure migrations: %w", err)
	}
	jobRepo := NewPostgresJobRepository(db)
	userRepo := NewPostgresUserRepository(db)
	analyticsRepo := NewPostgresAnalyticsRepository(db)

	cleanup := func() {
		_ = db.Close()
	}

	return Dependencies{
		DeploymentRepo: repo,
		JobRepo:        jobRepo,
		UserRepo:       userRepo,
		AnalyticsRepo:  analyticsRepo,
		DB:             db,
		HasDatabase:    true,
	}, cleanup, nil
}
