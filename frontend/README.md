# MiniDeploy Frontend

React/Vite frontend for the MiniDeploy public website, Guest Mode, and Admin dashboards.

The same production bundle is served by both Go listeners. Server-provided runtime metadata selects one of two modes:

```text
public         landing page, Guest Mode, and public Admin routing
private-admin  SSH emergency Admin routing
```

Within public mode, the browser pathname selects the landing page, `/guest/*`, or `/admin/*` experience.

## API Clients

The frontend uses separate API clients:

- `src/api/guest.js` exposes only `GET /api/guest/deployments`.
- `src/api/admin.js` uses `/api/admin/*` on the public listener.
- The same Admin client uses the original relative management paths on the private listener.

The frontend does not decide whether a user is authorized. Cloudflare Access and the Go server protect every public Admin page and API route. Guest safety also does not depend on hidden React controls: the guest API returns a dedicated server-side response containing only `app`, `url`, and `status`.

## Development

Install dependencies:

```bash
npm ci
```

Run the Vite development server:

```bash
npm run dev
```

Build the production bundle:

```bash
npm run build
```

Run tests and lint checks:

```bash
npm test
npm run lint
```

Production output is written to:

```text
dist/
```

The MiniDeploy Go server serves this directory in production.

## Features

- public MiniDeploy landing page
- no-login read-only Guest Mode
- Guest deployment status and public application links
- Cloudflare Access Admin Sign In path
- authenticated administrator identity display
- create deployments
- view deployment status and internal configuration
- open public application URLs
- view runtime logs
- view deployment logs
- view deployment history
- restart applications
- redeploy applications
- roll back applications
- delete deployments
- preserved SSH-only emergency dashboard behavior
