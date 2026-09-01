# MiniDeploy Architecture

## Overview

MiniDeploy is a single-server deployment platform running on Ubuntu Server.

The system separates public browsing, authenticated administration, emergency management, webhook automation, and application traffic.

```text
                              PUBLIC INTERNET
                                    |
                                    v
                            Cloudflare Edge
                                    |
                  +-----------------+------------------+
                  |                                    |
       minideploy.reactorlab.dev             *.reactorlab.dev
                  |                                    |
       +----------+-----------+                        |
       |                      |                        |
   /, /guest/*       /admin/*, /api/admin/*             |
     public          Cloudflare Access                  |
                     exact identity +                   |
                     biometric MFA                      |
       |                      |                        |
       +----------+-----------+                        |
                  |                                    |
                  v                                    v
          Cloudflare Tunnel                     Cloudflare Tunnel
                  |                                    |
                  +-----------------+------------------+
                                    |
                               Caddy :9002
                         +----------+----------+
                         |                     |
                         v                     v
                  127.0.0.1:9003        127.0.0.1:808x
                  MiniDeploy public       Docker apps
                  origin listener

GitHub -> webhook.reactorlab.dev -> Tunnel -> Caddy :9001
       -> 127.0.0.1:9000/webhooks/github -> HMAC validation

SSH key holder -> local port forward -> 127.0.0.1:9000
                 emergency full-management listener
```

Only the public landing page and Guest Mode are anonymous. Cloudflare Access protects the Admin paths, and the Go backend independently verifies the Access JWT before any admin handler runs.

Remote emergency management remains available using an SSH port forward:

```text
Operator browser
    |
localhost:9000
    |
SSH tunnel
    |
127.0.0.1:9000 on the server
    |
MiniDeploy private Admin dashboard/API
```

## Components

### Go Backend

The Go service is the control plane for MiniDeploy.

Responsibilities include:

- accepting deployment requests
- validating deployment configuration
- cloning Git repositories
- building Docker images
- allocating ports
- starting containers
- running health checks
- storing deployment metadata
- maintaining deployment history
- synchronizing Caddy routes
- collecting logs
- performing redeployments
- performing rollbacks
- processing GitHub webhooks

The process starts two independent HTTP servers:

```text
127.0.0.1:9000  private emergency management and GitHub webhook handlers
127.0.0.1:9003  public landing, Guest Mode, and Access-protected Admin routes
```

Both listeners are loopback-only. The public listener is reachable from the internet only through Cloudflare Tunnel and Caddy. Failure to construct the Access validator disables the public Admin routes while leaving the landing page, Guest Mode, webhook path, and SSH emergency plane available.

### React Dashboard

The React frontend is built with Vite. Production files are generated under `frontend/dist/` and served by both Go listeners.

The server writes a runtime-mode metadata value into the HTML response:

```text
public         landing, Guest Mode, or protected public Admin routing
private-admin  emergency port-9000 Admin routing
```

The public frontend uses separate clients:

- Guest Mode can call only `/api/guest/deployments`.
- Public Admin Mode calls only `/api/admin/*`.
- Private Admin Mode retains the original port-9000 paths.

Route selection affects presentation and API base paths only. Authorization is enforced by the Go handlers, not by hidden buttons or React state.

### Docker

Each application is built into a versioned Docker image.

Example image:

```text
minideploy-example:1788121778995838107
```

Containers use automatically allocated host ports.

Example:

```text
127.0.0.1:8081 -> container port 80
```

The host side is bound only to loopback.

This means a client on the local network cannot directly access:

```text
DELL_LAN_IP:8081
```

Public traffic must pass through the intended ingress path.

### Caddy

Caddy acts as MiniDeploy's reverse proxy.

The static Caddy configuration routes `minideploy.reactorlab.dev` to `127.0.0.1:9003` before importing generated application routes. MiniDeploy generates those application routes dynamically after deployment changes.

Two generated route files are maintained:

```text
caddy/apps.caddy
caddy/public-apps.caddy
```

Public routes use hostnames such as:

```text
example.reactorlab.dev
```

The reserved `minideploy` application label cannot be deployed, preventing generated routes from shadowing the public MiniDeploy site.

### Cloudflare Tunnel

Cloudflare Tunnel provides public connectivity without exposing inbound application ports on the router.

Current ingress is separated into:

```text
webhook.reactorlab.dev  -> Caddy :9001 -> 127.0.0.1:9000/webhooks/github
*.reactorlab.dev        -> Caddy :9002
```

Within `*.reactorlab.dev`, Caddy sends the MiniDeploy hostname to port 9003 and generated application hostnames to their loopback Docker ports.

Cloudflare Access applies only to these MiniDeploy paths:

```text
/admin
/admin/*
/api/admin
/api/admin/*
```

The landing page, Guest Mode, and guest API remain public. The legacy management API is intentionally absent from port 9003 and therefore from this ingress path.

## Deployment Flow

A new deployment normally requires only `repoUrl`; optional port, health, and
runtime environment values remain available through advanced Admin settings.

```text
Deploy request
     |
     v
Validate configuration
     |
     v
Derive application name
     |
     v
Clone Git repository
     |
     v
Inspect repository
     |
     +-- Dockerfile exists ----------------------+
     |                                           |
     |                                  Dockerfile strategy
     |                                  manual/default port
     |                                  manual/default health path
     |
     +-- no Dockerfile
             |
             v
      Ordered project detectors
             |
             +-- package.json + build script
             |   + Vite evidence
             |          |
             |          v
             |   vite-static strategy
             |   npm ci with package-lock.json
             |   npm install without a lockfile
             |   container port 80
             |   health path /
             |
             +-- conventional JavaScript npm project
             |   + nonempty scripts.start
             |   + Express runtime dependency
             |          |
             |          v
             |   node-express strategy
             |   npm ci/install
             |   npm start
             |   PORT=3000 by default
             |   health path /
             |
             +-- unsupported
                     |
                     v
             Clear detection error

Selected build strategy
     |
     v
Versioned Docker image build
     |
     v
Allocate host port
     |
     v
Start container
     |
     v
Startup validation
     |
     v
HTTP health check
     |
     v
Persist deployment metadata
     |
     v
Generate Caddy routes
     |
     v
Reload Caddy
     |
     v
Application live
```

Strategy selection is ordered and extensible. A repository-root `Dockerfile`
is authoritative, while later detectors can add supported project types without
placing runtime-specific logic in the deployment orchestrator. The Vite
detector reads `package.json` and known `vite.config.*` filenames; it does not
execute repository configuration merely to classify the project.

The `node-express` detector runs after Vite and requires npm, a nonempty start
script, and Express in runtime dependencies. It deliberately rejects
TypeScript indicators, Next.js, NestJS, Fastify, Koa, Bun, Deno, pnpm, and yarn.
The same boundary is revalidated when the persisted strategy is reused.

For `vite-static`, MiniDeploy writes a temporary generated Dockerfile outside
the cloned repository and uses the repository only as the Docker build context.
The build uses:

```text
node:24-alpine
npm ci --no-audit --no-fund        when package-lock.json is valid
npm install --no-audit --no-fund   when no package lock exists
npm run build -- --base=/ --outDir=dist
nginx:stable-alpine
```

Forcing the root base makes repositories with a development subpath suitable
for their generated ReactorLab hostname. The nginx runtime serves `dist/` on
container port 80 and falls back to `index.html` for unknown paths so client-side
SPA routes can load directly. Generated files never modify the Git worktree.

For `node-express`, MiniDeploy generates a fixed Node 24 Alpine Dockerfile in
the same service-owned temporary build area. It uses `npm ci` with a valid
`package-lock.json`, otherwise `npm install`, copies the application, and runs
`npm start`. No repository string becomes a Dockerfile instruction. MiniDeploy
injects `PORT` at runtime (3000 unless explicitly overridden), and the service
must honor `process.env.PORT`.

## Zero-Downtime Redeployment

Redeployments preserve the currently running version until the replacement is proven healthy.

```text
Current version
   |
   | remains live
   |
   +------------------------------+
                                  |
                                  v
                           Clone latest Git
                                  |
                                  v
                           Build new image
                                  |
                                  v
                       Start candidate container
                                  |
                                  v
                           Health checks
                                  |
                    +-------------+-------------+
                    |                           |
                  FAIL                         PASS
                    |                           |
                    v                           v
             Remove candidate          Update Caddy route
                    |                           |
                    v                           v
          Current stays live          New candidate live
                                                |
                                                v
                                     Retire old container
```

This protects the live deployment from:

- Git clone failures
- Docker build failures
- container startup failures
- HTTP health-check failures

## Rollback Flow

Rollback is also performed using a candidate container.

1. Load the most recent historical version.
2. Read that version's Docker image and deployment configuration.
3. Start it on a spare port.
4. Run startup and HTTP health checks.
5. Update deployment metadata.
6. Switch Caddy traffic.
7. Retire the previously active container.

Historical records preserve their own:

```text
containerPort
healthPath
strategy
packageManager
packageInstallMode
```

Legacy history entries fall back to the current deployment configuration.
Existing deployment records without a strategy are normalized to `dockerfile`,
preserving pre-zero-config behavior.

## Persistence

Deployment metadata is persisted to JSON files under:

```text
data/
```

The system maintains:

```text
data/deployments.json
data/deployment-history.json
data/deploy-logs/
```

Each deployment persists its selected strategy. Vite deployments also persist
the package manager and install mode, so manual redeploys and signed webhook
redeploys reuse the reviewed decision instead of reclassifying the repository.
Rollback history carries the same fields.

Docker containers use:

```text
--restart unless-stopped
```

The following services are enabled through systemd:

```text
docker
caddy
minideploy
cloudflared
systemd-timesyncd
```

The complete stack has been reboot-tested.

## GitHub Webhook Flow

```text
GitHub push
     |
     v
webhook.reactorlab.dev
     |
     v
Cloudflare Tunnel
     |
     v
Caddy webhook listener
     |
     v
POST /webhooks/github
     |
     v
HMAC-SHA256 validation
     |
     v
Repository matching
     |
     v
Zero-downtime redeploy
```

Only signed webhook requests are accepted for deployment processing.

## Security Boundaries

### Public Guest Plane

Anonymous visitors can reach:

```text
GET /
GET /guest/*
GET /api/guest/deployments
```

Guest data is constructed as `GuestDeploymentResponse`, not serialized from `DeploymentRecord`. Its JSON contract is exactly:

```text
app
url
status
```

There are no guest mutation, logs, history, repository, image, container, port, or health-configuration routes.

### Public Admin Plane

Cloudflare Access and the Go backend both guard:

```text
/admin
/admin/*
/api/admin
/api/admin/*
```

The backend accepts authority only from `Cf-Access-Jwt-Assertion` after cryptographic verification. A plain identity header, malformed token, wrong signature, wrong issuer, wrong audience, expired token, premature token, missing email, or non-matching email is rejected. Route-prefix protection also covers future admin subroutes before they reach the router.

### Emergency Management Plane

Private:

```text
127.0.0.1:9000
```

Remote access requires SSH key authentication and port forwarding. This listener retains the original full-management routes and remains available if Cloudflare Access, the tunnel, or the public listener is unavailable.

### GitHub Webhook Plane

The dedicated webhook hostname reaches only:

```text
POST /webhooks/github
```

The handler requires `X-Hub-Signature-256` to match an HMAC-SHA256 computed with the root-owned environment secret. Signed `ping` events return success; signed pushes to `main` are matched to an existing deployment before a zero-downtime redeploy is queued.

### Application Plane

Public application hostnames under `*.reactorlab.dev` enter through Cloudflare Tunnel and Caddy. Their Docker ports remain private:

```text
127.0.0.1:8081-8999
```

Docker application ports are not bound to LAN-facing interfaces.

Generated Vite builds receive neither MiniDeploy's environment nor secrets and
mount neither the Docker socket nor MiniDeploy internal paths. Their generated
Dockerfiles live in service-owned temporary storage outside the cloned Git
repository. Repository build scripts still execute code during image builds, so
the operator trust boundary remains reviewed repositories rather than arbitrary
multi-tenant input.

Runtime environment values are never supplied to Docker builds. At container
start, MiniDeploy writes the effective application environment to a temporary
mode-`0600` env-file, passes only that path to `docker run --env-file`, and
removes the temporary file when Docker returns. Node/Express containers also
receive the MiniDeploy-managed `PORT` value. Host publication remains bound to
`127.0.0.1`.

## HTTP Security

The MiniDeploy HTTP middleware applies protections including:

```text
X-Content-Type-Options
X-Frame-Options
Referrer-Policy
Permissions-Policy
Content-Security-Policy
Cache-Control
```

Cross-origin browser requests using state-changing methods are rejected. The webhook endpoint bypasses browser-origin checks only on the private listener because HMAC authentication is its trust mechanism; it is not registered on the public listener.

## Access JWT Validation

Access configuration is loaded only from:

```text
MINIDEPLOY_ACCESS_TEAM_DOMAIN
MINIDEPLOY_ACCESS_AUDIENCE
MINIDEPLOY_ACCESS_ADMIN_EMAIL
```

The configured team origin supplies the OIDC issuer and signing-key endpoint. The audience binds assertions to the MiniDeploy Access application. The normalized email claim must exactly match the configured administrator. If any setting is absent or invalid, public Admin requests fail closed.

## Secret and Configuration Storage

systemd loads runtime configuration from the root-owned `/etc/minideploy.env` file. It contains the three Access settings above and:

```text
MINIDEPLOY_GITHUB_WEBHOOK_SECRET
```

The environment file and any backup containing secret material must remain owned by `root:root` with mode `0600`. Values are never stored in the repository, frontend bundle, generated Caddy routes, deployment metadata, or documentation.

Access JWTs and Cloudflare session cookies are request credentials. MiniDeploy validates the assertion in memory and does not persist either credential.

Application runtime values use a separate store:

```text
/srv/minideploy/data/secrets/<app>.env
```

The containing directory is mode `0700`; each atomically replaced application
file is mode `0600`. Application names pass the normal validation and strict
child-path containment checks before read, replacement, or deletion. Values
cannot contain NUL or newlines in Phase 2, and variable names must match
`[A-Za-z_][A-Za-z0-9_]*`; `PORT` is reserved for MiniDeploy.

Deployment records persist only sorted configured variable names for the Admin
view. History and Guest DTOs contain no runtime environment metadata. Omitted
`environment` on redeploy preserves the secure file, while a supplied map is a
complete replacement. Candidate containers use the staged effective map; the
secure file changes only after health succeeds and is restored if metadata or
proxy cutover fails. Webhooks omit the field, restart uses Docker's existing
container configuration, and rollback loads the current secure map rather than
historical values. Deletion removes the contained application secret file.

Known values are removed from container log/error output before MiniDeploy
persists or returns it. The host operator/root/Docker-capable account is trusted
and can inherently inspect container environments; this boundary does not make
Docker secrets opaque to host administrators.

## systemd Hardening

The MiniDeploy systemd service uses restrictions including:

```text
NoNewPrivileges
PrivateTmp
PrivateDevices
ProtectSystem
ProtectKernelTunables
ProtectKernelModules
ProtectControlGroups
ProtectHostname
RestrictSUIDSGID
RestrictRealtime
LockPersonality
CapabilityBoundingSet
RestrictAddressFamilies
```

## Trust Model

```text
Public visitor
-> landing page
-> read-only Guest Mode
-> sanitized guest API only

Cloudflare Access authenticated administrator
+ exact email
+ biometric MFA
+ valid origin-verified Access JWT
-> full public Admin dashboard/API

SSH key holder
-> localhost:9000
-> emergency private full-management plane

GitHub
+ valid webhook HMAC
-> webhook endpoint only
```

These classes are intentionally non-interchangeable:

- UI controls do not grant authority.
- Guest and legacy paths are not aliases for Admin paths.
- Plain identity headers are ignored.
- Access tokens for another issuer, audience, identity, or validity window are rejected.
- Browser-origin checks remain in force for state-changing Admin requests.
- Webhook HMAC authentication grants access only to the webhook handler.

MiniDeploy is designed for repositories owned or reviewed by the server operator. It does not isolate untrusted users or Dockerfiles; Docker builds are effectively privileged, so multi-tenant sandboxing remains outside the v1 scope.
