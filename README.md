# MiniDeploy

MiniDeploy is a self-hosted deployment platform for containerized web applications.

It takes a Git repository, builds it into a Docker image, starts and health-checks a candidate container, routes traffic through Caddy, and publishes the application through a secure public URL.

MiniDeploy runs on a dedicated Ubuntu server and includes a private React dashboard, GitHub-triggered deployments, deployment history, zero-downtime redeploys and rollbacks, persistent logs, and failed-release protection.

## Why I Built It

MiniDeploy started as a way to understand what happens underneath deployment platforms such as Render, Railway, and Heroku.

Instead of relying on an existing platform, MiniDeploy implements the deployment workflow directly:

- clone source code from Git
- build Docker images
- manage application containers
- dynamically allocate ports
- perform startup and HTTP health checks
- update reverse-proxy routes
- expose applications through HTTPS
- process signed GitHub webhooks
- preserve deployment history
- reject unhealthy releases
- perform zero-downtime redeployments and rollbacks

The result is a small private deployment platform running on real Linux infrastructure.

## Features

### Git-Based Deployments

MiniDeploy accepts a repository URL, container port, and health-check path. It clones the repository, builds a versioned Docker image, starts the container on an available host port, validates health, persists metadata, and updates Caddy routing.

### Zero-Downtime Redeployments

Redeployments use a blue-green style workflow:

```text
Current version stays live
        |
        v
Build candidate image
        |
        v
Start candidate container
        |
        v
Run health checks
     /       \
   fail      pass
    |          |
Reject     Caddy cutover
candidate      |
              v
        Retire old container
```

If the candidate fails to build or becomes unhealthy, the existing deployment stays online.

### Zero-Downtime Rollbacks

MiniDeploy stores previous deployment versions and can restore an earlier Docker image using the same candidate-and-cutover strategy. Deployment history preserves the repository, image, container port, health path, previous host port, and deployment timestamp.

### GitHub Auto-Deploy

Signed GitHub push webhooks can trigger automatic redeployment of matching applications on pushes to `main`. Webhook signatures are validated with HMAC-SHA256.

### Logs

The dashboard exposes:

- **Runtime Logs** — output from the running container
- **Deploy Logs** — cloning, image builds, health checks, failures, proxy cutovers, redeployments, and rollbacks

### React Dashboard

The React/Vite dashboard supports:

- creating deployments
- viewing application status and configuration
- opening public application URLs
- runtime logs
- deployment logs
- deployment history
- restart
- redeploy
- rollback
- delete

The management interface stays private and is accessed through an SSH tunnel.

## Network Architecture

```text
Internet
   |
   v
Cloudflare
   |
   v
Cloudflare Tunnel
   |
   v
Caddy
   |
   v
127.0.0.1:808x
   |
   v
Docker Container
```

Applications receive first-level subdomains under `*.reactorlab.dev`, such as:

```text
https://example-app.reactorlab.dev
```

Managed Docker ports bind only to `127.0.0.1`, preventing LAN clients from bypassing Caddy and directly accessing application ports.

## Private Management Plane

The MiniDeploy API listens only on `127.0.0.1:9000`.

To access the dashboard remotely:

```bash
ssh -N -L 9000:127.0.0.1:9000 mini-server
```

Then open `http://localhost:9000`.

The management API is not exposed through the public Cloudflare application ingress.

## Technology

**Backend**
- Go
- Docker Engine
- systemd

**Frontend**
- React
- Vite
- JavaScript

**Infrastructure**
- Ubuntu Server 24.04 LTS
- Caddy
- Cloudflare Tunnel
- Cloudflare DNS and HTTPS
- GitHub Webhooks
- SSH
- UFW

## API

```text
GET    /health
POST   /deploy
GET    /deployments
GET    /deployments/{app}/logs
GET    /deployments/{app}/deploy-logs
GET    /deployments/{app}/history
POST   /deployments/{app}/restart
POST   /deployments/{app}/redeploy
POST   /deployments/{app}/rollback
DELETE /deployments/{app}
POST   /webhooks/github
```

## Reliability Testing

MiniDeploy has been tested for:

- normal deployments
- zero-downtime redeployments
- zero-downtime rollbacks
- traffic during proxy cutovers
- Docker build failures
- recovery after failed releases
- persistent deployment metadata
- full server reboots
- Docker, Caddy, and Cloudflare Tunnel recovery
- GitHub webhook delivery
- Go race conditions
- direct-LAN isolation of application ports

A failed candidate release does not replace the healthy live deployment.

## Security

MiniDeploy currently assumes deployed repositories are trusted.

Security measures include:

- localhost-only management API
- SSH-only remote dashboard access
- HMAC-authenticated GitHub webhooks
- Cloudflare Tunnel instead of public router port forwarding
- loopback-only Docker application bindings
- Caddy-controlled public ingress
- HTTP security headers
- cross-origin protections
- systemd service hardening
- UFW firewall
- candidate health validation before traffic cutover

Building arbitrary Dockerfiles is privileged, so MiniDeploy is not currently intended to run untrusted multi-tenant workloads.

## Development

Backend:

```bash
cd app
go test ./...
CGO_ENABLED=1 go test -race ./...
go build -o ../bin/minideploy .
```

Frontend:

```bash
cd frontend
npm install
npm run build
```

The Go server serves the production Vite bundle from `frontend/dist`.

## Status

MiniDeploy v1 is functionally complete. Remaining work is primarily documentation, cleanup, screenshots, and deployment of larger real-world applications for demonstration.

See `docs/architecture.md` and `docs/roadmap.md`.

MiniDeploy is part of the broader ReactorLab private developer infrastructure project.
