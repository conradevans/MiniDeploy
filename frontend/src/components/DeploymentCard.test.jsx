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
})
