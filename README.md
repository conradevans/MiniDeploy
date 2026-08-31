# MiniDeploy

MiniDeploy is a self-hosted deployment platform for containerized web applications.

It takes a Git repository, builds it into a Docker image, starts and health-checks a candidate container, routes traffic through Caddy, and publishes the application through a secure public URL.

MiniDeploy runs on a dedicated Ubuntu server and includes a public website, a sanitized read-only Guest Mode, a Cloudflare Access-protected Admin Mode, and an SSH-only emergency management plane. GitHub-triggered deployments, deployment history, zero-downtime redeploys and rollbacks, persistent logs, and failed-release protection remain part of the same Go service.

The production website is `https://minideploy.reactorlab.dev`.

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

### Zero-Config React/Vite Deployments

The Admin deploy form normally requires only a Git repository URL. After
cloning, MiniDeploy selects a conservative deployment strategy:

- A repository-root `Dockerfile` always uses the existing Dockerfile strategy
  and any advanced container-port or health-path settings.
- A repository without a Dockerfile is selected as `vite-static` only when it
  has `package.json`, a build script, and Vite build-system evidence.
- An npm lockfile selects `npm ci`; otherwise the Vite strategy uses
  `npm install`.

The Vite strategy generates a temporary multi-stage Dockerfile outside the
cloned repository. Node 24 builds the application with a root base and `dist`
output, then stable Alpine nginx serves it on port 80 with SPA fallback.
Generated files are not committed to or written into the source repository.
The selected strategy and npm install mode are persisted for redeploys,
webhooks, history, and rollbacks.

### GitHub Auto-Deploy

Signed GitHub push webhooks can trigger automatic redeployment of matching applications on pushes to `main`. Webhook signatures are validated with HMAC-SHA256.

### Public Website and Guest Mode

The public landing page explains MiniDeploy and provides two explicit entry paths:

- **Continue as Guest** opens a no-login, read-only view of published applications.
- **Admin Sign In** enters the Cloudflare Access authentication flow.

Guest Mode calls only `GET /api/guest/deployments`. The backend constructs a dedicated response containing exactly `app`, `url`, and `status`; repository URLs, container names, image names, ports, health paths, logs, history, and management controls are never serialized to guest clients.

### Admin Mode

The public Admin dashboard and `/api/admin/*` API are protected twice:

1. Cloudflare Access requires the configured administrator identity and biometric MFA.
2. MiniDeploy validates the Access JWT signature, issuer, audience, validity window, and exact normalized administrator email before invoking an admin handler.

Access configuration is supplied through environment variables. No account-specific Access values are stored in the repository.

### Logs

The dashboard exposes:

- **Runtime Logs** — output from the running container
- **Deploy Logs** — cloning, image builds, health checks, failures, proxy cutovers, redeployments, and rollbacks

### React Dashboard

The React/Vite frontend selects one of three server-defined experiences:

- public landing page
- read-only Guest Mode
- full Admin dashboard

Admin Mode supports:

- creating Dockerfile or zero-config React/Vite deployments
- viewing application status and configuration
- opening public application URLs
- runtime logs
- deployment logs
- deployment history
- restart
- redeploy
- rollback
- delete

The same Admin components use the protected `/api/admin/*` namespace on the public listener and the legacy relative API paths on the SSH-only listener. This frontend selection is not an authorization control; the Go server enforces every public admin route.

## Network Architecture

```text
Public browser
    |
    v
Cloudflare Edge
    |-- / and /guest/* -------------------------- public
    |-- /admin/* and /api/admin/* -- Access/MFA --+
    v                                               |
Cloudflare Tunnel                                  |
    v                                               |
Caddy :9002                                        |
    |-- minideploy.reactorlab.dev -> 127.0.0.1:9003+
    `-- <app>.reactorlab.dev ------> 127.0.0.1:808x

GitHub -- signed HMAC --> webhook.reactorlab.dev
    -> Cloudflare Tunnel -> Caddy :9001 -> 127.0.0.1:9000/webhooks/github

SSH key holder -> SSH port forward -> 127.0.0.1:9000
```

Applications receive first-level subdomains under `*.reactorlab.dev`, such as:

```text
https://example-app.reactorlab.dev
```

Managed Docker ports bind only to `127.0.0.1`, preventing LAN clients from bypassing Caddy and directly accessing application ports.

## Private Management Plane

The original full-management API continues to listen only on `127.0.0.1:9000`. It is the emergency backdoor and does not require Cloudflare to be available.

To access the dashboard remotely:

```bash
ssh -N -L 9000:127.0.0.1:9000 mini-server
```

Then open `http://localhost:9000`.

The legacy management routes are not registered on the public listener. Only the separately namespaced `/api/admin/*` handlers are public-facing, and those handlers require a valid Cloudflare Access JWT.

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

Public listener (`127.0.0.1:9003`):

```text
GET    /                              public landing page
GET    /guest/                        public Guest Mode
GET    /api/guest/deployments         sanitized public deployment list

GET    /admin/*                       Access-protected Admin SPA
GET    /api/admin/session             authenticated identity
POST   /api/admin/deploy
GET    /api/admin/deployments
GET    /api/admin/deployments/{app}/logs
GET    /api/admin/deployments/{app}/deploy-logs
GET    /api/admin/deployments/{app}/history
POST   /api/admin/deployments/{app}/restart
POST   /api/admin/deployments/{app}/redeploy
POST   /api/admin/deployments/{app}/rollback
DELETE /api/admin/deployments/{app}
```

Emergency private listener (`127.0.0.1:9000`):

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

The webhook endpoint is reachable publicly only through its dedicated Caddy/Cloudflare Tunnel hostname and still requires a valid HMAC signature.

### Guest Data Contract

Guest responses are an array of objects with exactly three fields:

```json
{
  "app": "example-app",
  "url": "https://example-app.reactorlab.dev",
  "status": "running"
}
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
- deterministic Dockerfile/Vite strategy selection
- strategy persistence across redeploy, webhook, history, and rollback paths

A failed candidate release does not replace the healthy live deployment.

## Security

MiniDeploy currently assumes deployed repositories are trusted.

Security measures include:

- distinct loopback-only listeners for the private and public-origin backends
- SSH-only emergency full-management access on port 9000
- Cloudflare Access protection for every public Admin page and API route
- biometric MFA enforced by the Access policy
- server-side Access JWT signature, issuer, audience, expiry, and exact-email validation
- fail-closed behavior when Access configuration is absent or invalid
- sanitized guest response types rather than serialization of internal deployment records
- no guest mutation, logs, history, repository, image, container, or port endpoints
- no legacy management routes on the public listener
- HMAC-authenticated GitHub webhooks
- Cloudflare Tunnel instead of public router port forwarding
- loopback-only Docker application bindings
- generated Vite Dockerfiles kept outside cloned repositories
- no Docker socket, MiniDeploy environment, secrets, or internal paths passed
  into generated application containers
- Caddy-controlled public ingress
- HTTP security headers
- cross-origin protections
- systemd service hardening
- UFW firewall
- candidate health validation before traffic cutover

The server does not trust plain Cloudflare identity headers. A cryptographically valid Access assertion is required. Cross-origin state-changing browser requests are rejected even when an Access session exists.

Repository builds execute code, and arbitrary Dockerfiles remain privileged. MiniDeploy is intended for operator-reviewed repositories rather than untrusted multi-tenant workloads.

## Development

Backend:

```bash
cd app
go test ./...
CGO_ENABLED=1 go test -race ./...
go vet ./...
go build -o ../bin/minideploy .
```

Frontend:

```bash
cd frontend
npm ci
npm test
npm run lint
npm run build
```

The Go server serves the production Vite bundle from `frontend/dist`.

## Status

MiniDeploy v1 is functionally complete, including the public landing page, Guest Mode, Access-protected Admin Mode, and SSH emergency path. Remaining work is optional presentation and future platform expansion.

See `docs/architecture.md` and `docs/roadmap.md`.

MiniDeploy is part of the broader ReactorLab private developer infrastructure project.
