# MiniDeploy Frontend

React/Vite management dashboard for MiniDeploy.

The frontend communicates with the MiniDeploy Go API using relative HTTP routes and runs on the same origin as the backend in production.

## Development

Install dependencies:

```bash
npm install
```

Run the Vite development server:

```bash
npm run dev
```

Build the production bundle:

```bash
npm run build
```

Production output is written to:

```text
dist/
```

The MiniDeploy Go server serves this directory in production.

## Features

- create deployments
- view deployment status
- open public ReactorLab URLs
- view runtime logs
- view deployment logs
- view deployment history
- restart applications
- redeploy applications
- roll back applications
- delete deployments
