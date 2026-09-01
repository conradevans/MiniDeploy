import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import DeploymentCard from './DeploymentCard'

afterEach(cleanup)

describe('DeploymentCard', () => {
  test('shows configured runtime names without rendering values', () => {
    const secretValue = ['NEVER', 'RENDER', 'THIS'].join('_')

    render(
      <DeploymentCard
        deployment={{
          app: 'express-app',
          repoUrl: 'https://github.com/example/express-app.git',
          image: 'minideploy-express-app:v1',
          port: 8081,
          containerPort: 3000,
          healthPath: '/',
          status: 'running',
          environmentVariables: ['JWT_SECRET', 'MONGODB_URI'],
          environment: {
            JWT_SECRET: secretValue,
          },
        }}
        busy={false}
        onLogs={vi.fn()}
        onDeployLogs={vi.fn()}
        onRestart={vi.fn()}
        onRedeploy={vi.fn()}
        onHistory={vi.fn()}
        onRollback={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    expect(screen.getByText('JWT_SECRET')).not.toBeNull()
    expect(screen.getByText('MONGODB_URI')).not.toBeNull()
    expect(screen.queryByText(secretValue)).toBeNull()
  })

  test('presents a full-stack deployment as one project with two services', () => {
    const secretValue = 'PHASE3_UI_SECRET_SENTINEL'

    render(
      <DeploymentCard
        deployment={{
          app: 'fullstack-app',
          repoUrl: 'https://github.com/example/fullstack-app.git',
          strategy: 'fullstack-vite-node',
          status: 'running',
          environmentVariables: ['ACCEPTANCE_MESSAGE'],
          environment: { ACCEPTANCE_MESSAGE: secretValue },
          services: [
            {
              name: 'frontend',
              path: 'frontend',
              strategy: 'vite-static',
              image: 'minideploy-fullstack-app-frontend:v1',
              port: 8081,
              containerPort: 80,
              healthPath: '/',
              packageInstallMode: 'ci',
              status: 'running',
            },
            {
              name: 'backend',
              path: 'backend',
              strategy: 'node-express',
              image: 'minideploy-fullstack-app-backend:v1',
              port: 8082,
              containerPort: 3000,
              healthPath: '/health',
              packageInstallMode: 'ci',
              status: 'running',
            },
          ],
        }}
        busy={false}
        onLogs={vi.fn()}
        onDeployLogs={vi.fn()}
        onRestart={vi.fn()}
        onRedeploy={vi.fn()}
        onHistory={vi.fn()}
        onRollback={vi.fn()}
        onDelete={vi.fn()}
      />,
    )

    expect(screen.getByText('Full-stack Vite + Node/Express')).not.toBeNull()
    expect(screen.getByText('Frontend')).not.toBeNull()
    expect(screen.getByText('Backend')).not.toBeNull()
    expect(screen.getByText('frontend/')).not.toBeNull()
    expect(screen.getByText('backend/')).not.toBeNull()
    expect(screen.getByText('ACCEPTANCE_MESSAGE')).not.toBeNull()
    expect(screen.queryByText(secretValue)).toBeNull()
    expect(screen.getAllByRole('link', { name: 'Open' })).toHaveLength(1)
  })
})
