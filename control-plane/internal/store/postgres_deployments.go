package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"MeshVPN-slef-hosting/control-plane/internal/domain"
	"MeshVPN-slef-hosting/control-plane/internal/logs"
)

type PostgresDeploymentRepository struct {
	db *sql.DB
}

func NewPostgresDeploymentRepository(db *sql.DB) *PostgresDeploymentRepository {
	return &PostgresDeploymentRepository{db: db}
}

func (r *PostgresDeploymentRepository) EnsureSchema() error {
	const schema = `
CREATE TABLE IF NOT EXISTS deployments (
    deployment_id TEXT PRIMARY KEY,
    repo TEXT NOT NULL,
    subdomain TEXT NOT NULL,
    port INTEGER NOT NULL,
    container TEXT,
    image TEXT,
    url TEXT,
    status TEXT NOT NULL,
    error TEXT,
    build_logs TEXT,
    env JSONB,
    build_args JSONB,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_deployments_started_at ON deployments(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
`

	_, err := r.db.Exec(schema)
	return err
}

func (r *PostgresDeploymentRepository) Start(rec domain.DeploymentRecord) {
	logs.Debugf("store-postgres", "start deployment deployment_id=%s status=%s", rec.DeploymentID, rec.Status)
	_ = r.upsert(rec)
}

func (r *PostgresDeploymentRepository) Update(rec domain.DeploymentRecord) {
	logs.Debugf("store-postgres", "update deployment deployment_id=%s status=%s", rec.DeploymentID, rec.Status)
	_ = r.upsert(rec)
}

func (r *PostgresDeploymentRepository) Get(id string) (domain.DeploymentRecord, error) {
	const query = `
SELECT deployment_id, requested_by, user_id, package, repo, subdomain, port, scaling_mode, min_replicas, max_replicas, cpu_target_utilization, cpu_request_milli, cpu_limit_milli, node_selector, cpu_cores, memory_mb, container, image, url, status, error, build_logs, env, build_args, started_at, finished_at
FROM deployments
WHERE deployment_id = $1
`

	var rec domain.DeploymentRecord
	var nodeSelectorRaw []byte
	var envRaw []byte
	var buildArgsRaw []byte
	var container sql.NullString
	var image sql.NullString
	var url sql.NullString
	var errText sql.NullString
	var buildLogs sql.NullString
	var finishedAt sql.NullTime
	var userID sql.NullString
	var pkg sql.NullString

	err := r.db.QueryRow(query, id).Scan(
		&rec.DeploymentID,
		&rec.RequestedBy,
		&userID,
		&pkg,
		&rec.Repo,
		&rec.Subdomain,
		&rec.Port,
		&rec.ScalingMode,
		&rec.MinReplicas,
		&rec.MaxReplicas,
		&rec.CPUTarget,
		&rec.CPURequest,
		&rec.CPULimit,
		&nodeSelectorRaw,
		&rec.CPUCores,
		&rec.MemoryMB,
		&container,
		&image,
		&url,
		&rec.Status,
		&errText,
		&buildLogs,
		&envRaw,
		&buildArgsRaw,
		&rec.StartedAt,
		&finishedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeploymentRecord{}, errors.New("deployment not found")
	}
	if err != nil {
		return domain.DeploymentRecord{}, fmt.Errorf("query deployment: %w", err)
	}

	if userID.Valid {
		rec.UserID = userID.String
	}
	if pkg.Valid {
		rec.Package = pkg.String
	}
	if container.Valid {
		rec.Container = container.String
	}
	if image.Valid {
		rec.Image = image.String
	}
	if url.Valid {
		rec.URL = url.String
	}
	if errText.Valid {
		rec.Error = errText.String
	}
	if buildLogs.Valid {
		rec.BuildLogs = buildLogs.String
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		rec.FinishedAt = &t
	}

	rec.NodeSelector = decodeStringMapJSON(nodeSelectorRaw)
	rec.Env = decodeStringMapJSON(envRaw)
	rec.BuildArgs = decodeStringMapJSON(buildArgsRaw)

	return rec, nil
}

func (r *PostgresDeploymentRepository) List() []domain.DeploymentRecord {
	const query = `
SELECT deployment_id, requested_by, user_id, package, repo, subdomain, port, scaling_mode, min_replicas, max_replicas, cpu_target_utilization, cpu_request_milli, cpu_limit_milli, node_selector, cpu_cores, memory_mb, container, image, url, status, error, build_logs, env, build_args, started_at, finished_at
FROM deployments
ORDER BY started_at DESC
`

	rows, err := r.db.Query(query)
	if err != nil {
		return []domain.DeploymentRecord{}
	}
	defer rows.Close()

	result := make([]domain.DeploymentRecord, 0)
	for rows.Next() {
		var rec domain.DeploymentRecord
		var nodeSelectorRaw []byte
		var envRaw []byte
		var buildArgsRaw []byte
		var container sql.NullString
		var image sql.NullString
		var url sql.NullString
		var errText sql.NullString
		var buildLogs sql.NullString
		var finishedAt sql.NullTime
		var userID sql.NullString
		var pkg sql.NullString

		err := rows.Scan(
			&rec.DeploymentID,
			&rec.RequestedBy,
			&userID,
			&pkg,
			&rec.Repo,
			&rec.Subdomain,
			&rec.Port,
			&rec.ScalingMode,
			&rec.MinReplicas,
			&rec.MaxReplicas,
			&rec.CPUTarget,
			&rec.CPURequest,
			&rec.CPULimit,
			&nodeSelectorRaw,
			&rec.CPUCores,
			&rec.MemoryMB,
			&container,
			&image,
			&url,
			&rec.Status,
			&errText,
			&buildLogs,
			&envRaw,
			&buildArgsRaw,
			&rec.StartedAt,
			&finishedAt,
		)
		if err != nil {
			continue
		}

		if userID.Valid {
			rec.UserID = userID.String
		}
		if pkg.Valid {
			rec.Package = pkg.String
		}
		if container.Valid {
			rec.Container = container.String
		}
		if image.Valid {
			rec.Image = image.String
		}
		if url.Valid {
			rec.URL = url.String
		}
		if errText.Valid {
			rec.Error = errText.String
		}
		if buildLogs.Valid {
			rec.BuildLogs = buildLogs.String
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			rec.FinishedAt = &t
		}

		rec.NodeSelector = decodeStringMapJSON(nodeSelectorRaw)
		rec.Env = decodeStringMapJSON(envRaw)
		rec.BuildArgs = decodeStringMapJSON(buildArgsRaw)

		result = append(result, rec)
	}

	return result
}

// ListByUserID returns all deployments for a specific user
func (r *PostgresDeploymentRepository) ListByUserID(userID string) []domain.DeploymentRecord {
	const query = `
SELECT deployment_id, requested_by, user_id, package, repo, subdomain, port, scaling_mode, min_replicas, max_replicas, cpu_target_utilization, cpu_request_milli, cpu_limit_milli, node_selector, cpu_cores, memory_mb, container, image, url, status, error, build_logs, env, build_args, started_at, finished_at
FROM deployments
WHERE user_id = $1
ORDER BY started_at DESC
`

	rows, err := r.db.Query(query, userID)
	if err != nil {
		logs.Errorf("store-postgres", "query user deployments failed user_id=%s err=%v", userID, err)
		return []domain.DeploymentRecord{}
	}
	defer rows.Close()

	result := make([]domain.DeploymentRecord, 0)
	for rows.Next() {
		var rec domain.DeploymentRecord
		var nodeSelectorRaw []byte
		var envRaw []byte
		var buildArgsRaw []byte
		var container sql.NullString
		var image sql.NullString
		var url sql.NullString
		var errText sql.NullString
		var buildLogs sql.NullString
		var finishedAt sql.NullTime
		var uid sql.NullString
		var pkg sql.NullString

		err := rows.Scan(
			&rec.DeploymentID,
			&rec.RequestedBy,
			&uid,
			&pkg,
			&rec.Repo,
			&rec.Subdomain,
			&rec.Port,
			&rec.ScalingMode,
			&rec.MinReplicas,
			&rec.MaxReplicas,
			&rec.CPUTarget,
			&rec.CPURequest,
			&rec.CPULimit,
			&nodeSelectorRaw,
			&rec.CPUCores,
			&rec.MemoryMB,
			&container,
			&image,
			&url,
			&rec.Status,
			&errText,
			&buildLogs,
			&envRaw,
			&buildArgsRaw,
			&rec.StartedAt,
			&finishedAt,
		)
		if err != nil {
			continue
		}

		if uid.Valid {
			rec.UserID = uid.String
		}
		if pkg.Valid {
			rec.Package = pkg.String
		}
		if container.Valid {
			rec.Container = container.String
		}
		if image.Valid {
			rec.Image = image.String
		}
		if url.Valid {
			rec.URL = url.String
		}
		if errText.Valid {
			rec.Error = errText.String
		}
		if buildLogs.Valid {
			rec.BuildLogs = buildLogs.String
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			rec.FinishedAt = &t
		}

		rec.NodeSelector = decodeStringMapJSON(nodeSelectorRaw)
		rec.Env = decodeStringMapJSON(envRaw)
		rec.BuildArgs = decodeStringMapJSON(buildArgsRaw)

		result = append(result, rec)
	}

	logs.Debugf("store-postgres", "list user deployments user_id=%s count=%d", userID, len(result))
	return result
}

func (r *PostgresDeploymentRepository) upsert(rec domain.DeploymentRecord) error {
	const stmt = `
INSERT INTO deployments (
	deployment_id, requested_by, user_id, package, repo, subdomain, port, scaling_mode, min_replicas, max_replicas, cpu_target_utilization, cpu_request_milli, cpu_limit_milli, node_selector, cpu_cores, memory_mb, container, image, url, status, error, build_logs, env, build_args, started_at, finished_at
)
VALUES (
	$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, NULLIF($8, ''), $9, $10, $11, $12, $13, $14::jsonb, $15, $16, NULLIF($17, ''), NULLIF($18, ''), NULLIF($19, ''), $20, NULLIF($21, ''), NULLIF($22, ''), $23::jsonb, $24::jsonb, $25, $26
)
ON CONFLICT (deployment_id)
DO UPDATE SET
	requested_by = EXCLUDED.requested_by,
	user_id = EXCLUDED.user_id,
	package = EXCLUDED.package,
    repo = EXCLUDED.repo,
    subdomain = EXCLUDED.subdomain,
    port = EXCLUDED.port,
	scaling_mode = EXCLUDED.scaling_mode,
	min_replicas = EXCLUDED.min_replicas,
	max_replicas = EXCLUDED.max_replicas,
	cpu_target_utilization = EXCLUDED.cpu_target_utilization,
	cpu_request_milli = EXCLUDED.cpu_request_milli,
	cpu_limit_milli = EXCLUDED.cpu_limit_milli,
	node_selector = EXCLUDED.node_selector,
	cpu_cores = EXCLUDED.cpu_cores,
	memory_mb = EXCLUDED.memory_mb,
    container = EXCLUDED.container,
    image = EXCLUDED.image,
    url = EXCLUDED.url,
    status = EXCLUDED.status,
    error = EXCLUDED.error,
    build_logs = EXCLUDED.build_logs,
    env = EXCLUDED.env,
    build_args = EXCLUDED.build_args,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at
`

	nodeSelectorJSON := encodeStringMapJSON(rec.NodeSelector)
	envJSON := encodeStringMapJSON(rec.Env)
	buildArgsJSON := encodeStringMapJSON(rec.BuildArgs)

	_, err := r.db.Exec(
		stmt,
		rec.DeploymentID,
		rec.RequestedBy,
		rec.UserID,
		rec.Package,
		rec.Repo,
		rec.Subdomain,
		rec.Port,
		rec.ScalingMode,
		rec.MinReplicas,
		rec.MaxReplicas,
		rec.CPUTarget,
		rec.CPURequest,
		rec.CPULimit,
		nodeSelectorJSON,
		rec.CPUCores,
		rec.MemoryMB,
		rec.Container,
		rec.Image,
		rec.URL,
		rec.Status,
		rec.Error,
		rec.BuildLogs,
		envJSON,
		buildArgsJSON,
		rec.StartedAt,
		rec.FinishedAt,
	)

	return err
}

func encodeStringMapJSON(values map[string]string) string {
	if len(values) == 0 {
		return "{}"
	}

	data, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}

	return string(data)
}

func decodeStringMapJSON(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}

	result := make(map[string]string)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	if len(result) == 0 {
		return nil
	}

	return result
}
