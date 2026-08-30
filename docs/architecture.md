# MiniDeploy Architecture

## Overview

MiniDeploy is a single-server deployment platform running on Ubuntu Server.

The system separates the private management plane from public application traffic.

```text
                         PUBLIC INTERNET
                               |
                               v
                       Cloudflare Edge
                               |
                               v
                      Cloudflare Tunnel
                               |
                   +-----------+-----------+
                   |                       |
                   v                       v
           webhook.reactorlab.dev   *.reactorlab.dev
                   |                       |
                   v                       v
             Caddy :9001              Caddy :9002
                   |                       |
                   v                       |
          127.0.0.1:9000                  |
             MiniDeploy                   |
                 Go API                   |
                   |                       |
                   +-----------+-----------+
                               |
                               v
                         Caddy routing
                               |
                    +----------+----------+
                    |                     |
                    v                     v
             127.0.0.1:8081        127.0.0.1:8082
                    |                     |
                    v                     v
             Docker App A          Docker App B
```

The dashboard is not exposed through the public Cloudflare ingress.

Remote management is performed using an SSH port forward:

```text
Mac browser
    |
localhost:9000
    |
SSH tunnel
    |
127.0.0.1:9000 on the Dell
    |
MiniDeploy
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

The HTTP server listens exclusively on:

```text
127.0.0.1:9000
```

### React Dashboard

The React dashboard is built with Vite.

Production files are generated under:

```text
frontend/dist/
```

The Go service serves the production React bundle.

The frontend communicates with the Go API using relative URLs so the UI and API remain on the same origin.

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

MiniDeploy generates routing configuration dynamically after deployment changes.

Two generated route files are maintained:

```text
caddy/apps.caddy
caddy/public-apps.caddy
```

Public routes use hostnames such as:

```text
example.reactorlab.dev
```

### Cloudflare Tunnel

Cloudflare Tunnel provides public connectivity without exposing inbound application ports on the router.

Current ingress is separated into:

```text
webhook.reactorlab.dev
```

for GitHub webhook traffic and:

```text
*.reactorlab.dev
```

for deployed applications.

The management API is intentionally excluded.

## Deployment Flow

A new deployment follows this process:

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
Docker build
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
```

Legacy history entries fall back to the current deployment configuration.

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

### Management Plane

Private:

```text
127.0.0.1:9000
```

Remote access requires SSH tunneling.

### Application Plane

Public:

```text
*.reactorlab.dev
```

Traffic enters through Cloudflare Tunnel and Caddy.

### Docker Plane

Private:

```text
127.0.0.1:8081-8999
```

Docker application ports are not bound to LAN-facing interfaces.

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

Cross-origin browser requests using state-changing methods are rejected except for the authenticated GitHub webhook endpoint.

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

MiniDeploy is designed for trusted repositories owned or reviewed by the server operator.

It does not currently isolate untrusted users or untrusted Dockerfiles.

A malicious Docker build could compromise the server because Docker access is effectively privileged.

Multi-tenant sandboxing is outside the v1 scope.
