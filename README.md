# MeshVPN Self-Hosting POC

This Phase 1-3 proof of concept turns the control plane into a small deployment engine:

1. Receive a deployment request over HTTP.
2. Clone a Git repository into `apps/`.
3. Build a Docker image from that repository.
4. Run the container on a shared Traefik network.
5. Route `<subdomain>.localhost` to the deployed app.

## Why each piece exists

- `control-plane/`: the API that accepts deployment requests and orchestrates clone/build/run.
- `apps/`: local checkout area for cloned repositories.
- `infra/docker-compose.yml`: runs Traefik, which gives you stable hostnames instead of random high ports.
- `worker/`: reserved for a later async worker; in this POC the control plane does the work synchronously.

## Prerequisites

- Go installed and available in `PATH`
- Git installed and available in `PATH`
- Docker Desktop installed and running

This machine already has Go, Git, and Docker CLI installed. Docker Desktop still needs to be started before deployments will work.

## Why the request includes `port`

Traefik must know which port the application listens on inside its container. Many web apps use `3000`, but some use `80`, `8080`, or something else. The `port` field lets the control plane tell Traefik the correct target.

## Run the POC (Localhost)

1. Start Docker Desktop.
2. Start Traefik:

   ```powershell
   cd infra
   docker compose up -d
   ```

3. Start the Go API:

   ```powershell
   cd control-plane
   go run ./cmd/control-plane
   ```

4. Check health:

   ```powershell
   curl http://localhost:8080/health
   ```

5. Deploy a repo that contains a root-level `Dockerfile`:

   ```powershell
   curl -X POST http://localhost:8080/deploy ^
     -H "Content-Type: application/json" ^
     -d "{\"repo\":\"https://github.com/your-org/your-app.git\",\"port\":3000,\"subdomain\":\"demo\"}"
   ```

6. Open the deployed app at `http://demo.localhost`.

7. Open the Traefik dashboard at `http://localhost:8081`.

## Run With Cloudflare Tunnel (Public)

1. Copy `infra/.env.example` to `infra/.env` and set:
   - `CLOUDFLARE_TUNNEL_TOKEN=<your token>`
   - `APP_BASE_DOMAIN=keshavstack.tech`

2. In Cloudflare Zero Trust, for your named tunnel, add these Public Hostnames:
   - `self.keshavstack.tech` -> `http://control-plane:8080`
   - `*.keshavstack.tech` -> `http://traefik:80`

   This is required because `cloudflared` runs inside Docker, where `localhost` means the `cloudflared` container itself.

3. Start the full stack:

   ```powershell
   cd infra
   docker compose --env-file .env up -d
   ```

4. Verify control plane:

   ```powershell
   curl https://self.keshavstack.tech/health
   ```

5. Deploy an app with a chosen subdomain:

   ```powershell
   curl -X POST https://self.keshavstack.tech/deploy ^
     -H "Content-Type: application/json" ^
     -d "{\"repo\":\"https://github.com/your-org/your-app.git\",\"port\":3000,\"subdomain\":\"projectname\"}"
   ```

6. Access deployed app at:
   - `https://projectname.keshavstack.tech`

## Build Logs And App Logs API

The deploy API now stores build logs and exposes container runtime logs.

### Deploy with runtime env and build args

Use `env` for container runtime environment variables and `build_args` for Docker build-time arguments.

```powershell
curl -X POST https://self.keshavstack.tech/deploy ^
   -H "Content-Type: application/json" ^
   -d "{\"repo\":\"https://github.com/your-org/your-app.git\",\"port\":3000,\"subdomain\":\"projectname\",\"env\":{\"API_URL\":\"https://api.example.com\",\"NODE_ENV\":\"production\"},\"build_args\":{\"NEXT_PUBLIC_API_BASE\":\"https://api.example.com\"}}"
```

Response now includes `build_logs`.

### Read deployment history

```powershell
curl https://self.keshavstack.tech/deployments
```

### Read build logs by deployment id

```powershell
curl https://self.keshavstack.tech/deployments/<deployment_id>/build-logs
```

### Read application logs by deployment id

```powershell
curl "https://self.keshavstack.tech/deployments/<deployment_id>/app-logs?tail=300"
```

`tail` is optional and defaults to `200`.

### Dockerfile note for build args

Build args work only if the target Dockerfile declares matching `ARG` keys.

```dockerfile
ARG NEXT_PUBLIC_API_BASE
ENV NEXT_PUBLIC_API_BASE=$NEXT_PUBLIC_API_BASE
```

## Current POC limits

- The repo must have a `Dockerfile` in its root.
- The deploy endpoint is synchronous, so large builds will keep the HTTP request open.
- There is no persistence layer yet, so deployment state only exists in Docker and the local filesystem.
- There is no cleanup endpoint yet.

Those limits are normal for this stage. They are the reason a later worker, job queue, and database layer will exist.

## Phase 1 Backend Infra (Implemented)

The control plane now includes:

- Supabase JWT auth middleware (GitHub provider only).
- Async deployment queue with background worker.
- Postgres/Supabase migrations for org/project/deployments/logs/analytics tables.

### New environment variables

- `DATABASE_URL` or `SUPABASE_DB_URL`: Postgres connection string.
- `SUPABASE_JWT_SECRET`: JWT secret from Supabase project settings.
- `REQUIRE_AUTH`: `true` (default) or `false` for local debugging.
- `WORKER_POLL_INTERVAL`: e.g. `2s`.
- `WORKER_BATCH_SIZE`: reserved for next worker optimization phase.

Set these in `infra/.env` (copy from `infra/.env.example`), then run:

```powershell
cd infra
docker compose --env-file .env up -d
```

### Deploy API behavior change

`POST /deploy` now returns `202 Accepted` with `status: queued`.
The background worker performs clone/build/run and updates deployment status to `deploying`, then `running` or `failed`.

## Postman Testing

Complete Phase 1 Postman flow is documented in:

- `docs/postman-phase1-api-testing.md`