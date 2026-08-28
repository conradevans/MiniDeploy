# MiniDeploy Architecture

MiniDeploy will run on a dedicated Linux server.

```text
                         Internet
                             |
                             v
                       Reverse Proxy
                             |
               +-------------+-------------+
               |             |             |
               v             v             v
             App A         App B         App C
           Container     Container     Container

                             ^
                             |
                       Docker Engine
                             ^
                             |
                     MiniDeploy Backend
                             ^
                             |
                         Dashboard
```

## Deployment Flow

A deployment will eventually follow this process:

1. Receive a Git repository.
2. Clone or pull the repository.
3. Build a Docker image.
4. Create a container from the image.
5. Start the container.
6. Register the application with the reverse proxy.
7. Perform a health check.
8. Mark the deployment as successful.

This architecture is intentionally **conceptual** right now.
