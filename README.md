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

## Run the POC

1. Start Docker Desktop.
2. Start Traefik:

   ```powershell
   cd infra
   docker compose up -d
   ```

3. Start the Go API:

   ```powershell
   cd control-plane
   go run .
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

## Current POC limits

- The repo must have a `Dockerfile` in its root.
- The deploy endpoint is synchronous, so large builds will keep the HTTP request open.
- There is no persistence layer yet, so deployment state only exists in Docker and the local filesystem.
- There is no cleanup endpoint yet.

Those limits are normal for this stage. They are the reason a later worker, job queue, and database layer will exist.