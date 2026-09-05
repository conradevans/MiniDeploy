import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import DeploymentCard from './DeploymentCard'

afterEach(cleanup)

function actions() {
  return {
    onLogs: vi.fn(),
    onDeployLogs: vi.fn(),
    onRestart: vi.fn(),
    onRedeploy: vi.fn(),
    onHistory: vi.fn(),
    onRollback: vi.fn(),
    onDelete: vi.fn(),
  }
}

function deployment(overrides = {}) {
  return {
    app: 'express-app',
    repoUrl: 'https://github.com/example/express-app.git',
    strategy: 'node-express',
    packageInstallMode: 'ci',
    containerPort: 3000,
    healthPath: '/',
    status: 'running',
    ...overrides,
  }
}

describe('DeploymentCard MiniBase section', () => {
  test('keeps an unattached supported deployment read-only', () => {
    render(
      <DeploymentCard
        deployment={deployment()}
        busy={false}
        {...actions()}
      />,
    )

    expect(screen.getByText('No database attached')).toBeTruthy()

    expect(
      screen.getByText(
        'Attach or manage a PostgreSQL database from MiniBase.',
      ),
    ).toBeTruthy()

    expect(
      screen.queryByRole('button', {
        name: 'Add MiniBase Database',
      }),
    ).toBeNull()
  })

  test('directs detached deployments back to MiniBase', () => {
    render(
      <DeploymentCard
        deployment={deployment({
          status: 'database-detached',
          databaseDetached: true,
        })}
        busy={false}
        {...actions()}
      />,
    )

    expect(screen.getByText('Database detached')).toBeTruthy()

    expect(
      screen.getByText(
        'Reconnect this deployment from MiniBase.',
      ),
    ).toBeTruthy()

    expect(
      screen.queryByRole('button', {
        name: 'Add MiniBase Database',
      }),
    ).toBeNull()
  })

  test('shows attached safe metadata without lifecycle controls', () => {
    const view = render(
      <DeploymentCard
        deployment={deployment({
          databaseAttachments: [
            {
              attachmentId:
                'attachment_0123456789abcdef0123456789abcdef',
              databaseId:
                'database_0123456789abcdef0123456789abcdef',
              displayName: 'Express Production',
              bindingName: 'primary',
              password: 'mock-password-must-not-render',
              databaseUrl: 'postgresql://must-not-render',
            },
          ],
        })}
        busy={false}
        {...actions()}
      />,
    )

    expect(screen.getByText('Express Production')).toBeTruthy()

    expect(
      screen.getByText(
        'Ready · Primary binding · Managed in MiniBase',
      ),
    ).toBeTruthy()

    expect(
      screen.queryByRole('button', {
        name: 'Add MiniBase Database',
      }),
    ).toBeNull()

    expect(view.container.textContent).not.toContain(
      'mock-password-must-not-render',
    )

    expect(view.container.textContent).not.toContain(
      'postgresql://',
    )
  })

  test('still explains unsupported deployment strategies', () => {
    render(
      <DeploymentCard
        deployment={deployment({
          strategy: 'vite-static',
          containerPort: 80,
        })}
        busy={false}
        {...actions()}
      />,
    )

    expect(
      screen.getByText(
        'Database attachment is unavailable for this deployment strategy.',
      ),
    ).toBeTruthy()

    expect(
      screen.queryByRole('button', {
        name: 'Add MiniBase Database',
      }),
    ).toBeNull()
  })
})
