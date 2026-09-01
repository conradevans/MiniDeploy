# MiniDeploy Roadmap

## v1 — Core Platform

### Server Infrastructure

- [x] Install Ubuntu Server
- [x] Configure SSH
- [x] Configure headless operation
- [x] Configure firewall
- [x] Install Docker Engine
- [x] Configure systemd services
- [x] Configure reliable NTP synchronization

### Deployment Engine

- [x] Deploy from Git repositories
- [x] Build Docker images automatically
- [x] Dynamically allocate host ports
- [x] Support custom container ports
- [x] Support custom HTTP health paths
- [x] Persist deployment metadata
- [x] Detect Dockerfile and zero-config React/Vite deployment strategies
- [x] Package Vite production bundles in a generated static runtime
- [x] Detect conventional JavaScript Node/Express services conservatively
- [x] Package Node/Express services in a generated npm runtime
- [x] Detect exact `frontend/` Vite + `backend/` Express full-stack projects
- [x] Build and manage paired frontend/backend releases as one project
- [x] Persist runtime environment values separately with explicit lifecycle semantics
- [x] Persist deployment strategy for redeploy, webhook, history, and rollback
- [x] Restart applications
- [x] Delete applications

### Networking

- [x] Configure Caddy
- [x] Generate reverse-proxy routes automatically
- [x] Configure ReactorLab domain
- [x] Configure Cloudflare Tunnel
- [x] Publish applications through `*.reactorlab.dev`
- [x] Configure automatic HTTPS
- [x] Restrict application ports to loopback
- [x] Publish `minideploy.reactorlab.dev`
- [x] Separate public-origin traffic onto loopback port 9003
- [x] Route full-stack `/api` traffic to the backend on one project hostname
- [x] Use dedicated labeled Docker bridge networks per full-stack release

### Reliability

- [x] Startup health validation
- [x] HTTP health validation
- [x] Reject failed candidate releases
- [x] Zero-downtime redeployment
- [x] Deployment history
- [x] Zero-downtime rollback
- [x] Preserve historical deployment configuration
- [x] Recover services after server reboot
- [x] Recover Docker applications after server reboot
- [x] Test failed-build recovery
- [x] Test live traffic during proxy cutover
- [x] Require both full-stack services to pass health checks before cutover
- [x] Redeploy and roll back full-stack service pairs atomically

### GitHub Automation

- [x] GitHub webhook endpoint
- [x] HMAC-SHA256 webhook validation
- [x] Automatic redeployment on pushes to `main`
- [x] Cloudflare webhook ingress
- [x] Keep legacy management routes off the public listener

### Observability

- [x] Runtime container logs
- [x] Persistent deployment activity logs
- [x] Include Docker build failures in deployment logs
- [x] Deployment status dashboard

### Dashboard

- [x] Initial HTML dashboard
- [x] React/Vite dashboard
- [x] Deploy applications
- [x] Deploy standard React/Vite repositories from only a repository URL
- [x] Deploy conventional Node/Express repositories from only a repository URL
- [x] Deploy conventional full-stack repositories from one repository URL
- [x] Present full-stack services as one Admin project with paired history
- [x] Keep Dockerfile port and health settings behind advanced controls
- [x] Add masked runtime environment variable controls and names-only status
- [x] View application status
- [x] View public URLs
- [x] Runtime logs
- [x] Deployment logs
- [x] Deployment history
- [x] Restart
- [x] Redeploy
- [x] Rollback
- [x] Delete
- [x] Public landing page
- [x] Read-only Guest Mode
- [x] Cloudflare Access Admin Mode

### Security

- [x] Bind management API to localhost
- [x] SSH-only management access
- [x] HTTP security headers
- [x] Cross-origin protections
- [x] Signed GitHub webhooks
- [x] systemd service hardening
- [x] UFW firewall
- [x] Loopback-only Docker application bindings
- [x] Verify direct LAN access to app ports is blocked
- [x] Preserve SSH emergency management on loopback port 9000
- [x] Protect all public Admin paths with Cloudflare Access
- [x] Require biometric MFA for the Admin Access policy
- [x] Verify Access JWT signature, issuer, audience, validity, and exact email
- [x] Reject plain identity-header spoofing
- [x] Serialize guest data through a dedicated allowlisted response type
- [x] Keep guest mutations, logs, and history unavailable
- [x] Keep runtime environment names and values out of Guest Mode and history
- [x] Inject runtime values with temporary Docker env-files
- [x] Keep full-stack runtime environment values backend-only
- [x] Validate full-stack paths, symlinks, and Docker resource ownership

### Testing

- [x] Core Go unit tests
- [x] Deployment configuration tests
- [x] History tests
- [x] Security middleware tests
- [x] GitHub signature tests
- [x] Loopback binding tests
- [x] Go race detector
- [x] Failed-build recovery test
- [x] Reboot persistence test
- [x] Public routing test
- [x] Guest serialization and route-isolation tests
- [x] Public Admin authorization matrix
- [x] Route-manipulation regression tests
- [x] React routing and Guest Mode tests
- [x] Deployment-strategy detection and false-positive tests
- [x] Vite package-lock, generated runtime, and SPA-fallback tests
- [x] Zero-config Admin form serialization tests
- [x] Full-stack detection, lifecycle, routing, isolation, and leak tests

## v1 Final Polish

- [x] Audit and clean stale repository-local backup files
- [x] Update final architecture and security documentation
- [ ] Improve deployment naming readability
- [ ] Review Docker image cleanup strategy
- [ ] Add screenshots to README
- [ ] Record a short end-to-end demo
- [x] Deploy a larger real application through MiniDeploy
- [ ] Final fresh-server setup documentation

## Possible v2 Features

These are intentionally outside the initial v1 scope.

### Build System

- [ ] Repository-specific build configuration
- [ ] Build arguments
- [ ] Environment-variable management
- [ ] Build cancellation
- [ ] Deployment queues
- [ ] Build timeouts
- [ ] Streaming build logs

### Application Management

- [ ] Custom application names
- [ ] Custom domains
- [ ] Environment-variable editor
- [ ] Secrets management
- [ ] Application resource limits
- [ ] CPU and memory metrics
- [ ] Deployment retention controls

### Git Integration

- [ ] Multiple branches
- [ ] Pull-request preview deployments
- [ ] GitHub installation/app integration
- [ ] Commit metadata in deployment history
- [ ] Commit-based rollback selection

### Reliability

- [ ] Automated rollback after post-cutover health degradation
- [ ] Scheduled health monitoring
- [ ] Deployment cancellation
- [ ] Graceful application drain before container retirement

### Platform Expansion

MiniDeploy is intended to become one component of the broader ReactorLab private developer cloud.

Potential companion systems include:

```text
MiniDeploy   application deployment
MiniBase     managed private databases
MiniAI       local development / infrastructure assistant
```

A future unified dashboard could manage all ReactorLab services.
