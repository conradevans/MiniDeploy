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

### GitHub Automation

- [x] GitHub webhook endpoint
- [x] HMAC-SHA256 webhook validation
- [x] Automatic redeployment on pushes to `main`
- [x] Cloudflare webhook ingress
- [x] Keep management API private

### Observability

- [x] Runtime container logs
- [x] Persistent deployment activity logs
- [x] Include Docker build failures in deployment logs
- [x] Deployment status dashboard

### Dashboard

- [x] Initial HTML dashboard
- [x] React/Vite dashboard
- [x] Deploy applications
- [x] View application status
- [x] View public URLs
- [x] Runtime logs
- [x] Deployment logs
- [x] Deployment history
- [x] Restart
- [x] Redeploy
- [x] Rollback
- [x] Delete

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

## v1 Final Polish

- [ ] Clean stale local backup files
- [ ] Improve deployment naming readability
- [ ] Review Docker image cleanup strategy
- [ ] Add screenshots to README
- [ ] Record a short end-to-end demo
- [ ] Deploy a larger real application through MiniDeploy
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
