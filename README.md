# MiniDeploy

MiniDeploy is a self-hosted deployment platform for containerized web applications.

The goal is to build a lightweight deployment system that can take an application from a Git repository, build it into a Docker image, run it as a container, and expose it through a simple deployment interface.

## Goals

MiniDeploy will eventually support:

- Deploying applications from Git repositories
- Building Docker images automatically
- Running and managing application containers
- Viewing deployment status and logs
- Assigning ports and domains to applications
- Redeploying applications
- Health checks
- Automatic deployments
- Rollbacks

## Initial Architecture

```text
Git Repository
      |
      v
MiniDeploy
      |
      v
Clone Repository
      |
      v
Docker Build
      |
      v
Docker Container
      |
      v
Reverse Proxy
      |
      v
Application
```

## Development Environment

MiniDeploy will run on a dedicated Ubuntu Linux server using Docker.

The project is being built incrementally to explore:

- Linux server administration
- Docker
- Networking
- Reverse proxies
- Deployment automation
- Backend development
- Infrastructure and DevOps concepts
