import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import DeploymentCard from './DeploymentCard'

afterEach(cleanup)

function actions(overrides = {}) {
  return {
    onLogs: vi.fn(),
    onDeployLogs: vi.fn(),
    onRestart: vi.fn(),
    onRedeploy: vi.fn(),
    onHistory: vi.fn(),
    onRollback: vi.fn(),
    onDelete: vi.fn(),
    onDatabase: vi.fn(),
    ...overrides,
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
  test('opens the add-database flow for a supported unattached deployment', () => {
    const callbacks = actions()
    const record = deployment()
    render(<DeploymentCard deployment={record} busy={false} {...callbacks} />)
    expect(screen.getByText('No database attached')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Add MiniBase Database' }))
    expect(callbacks.onDatabase).toHaveBeenCalledWith(record)
  })

  test('shows attached safe metadata and never renders injected credential fields', () => {
    const view = render(
      <DeploymentCard
        deployment={deployment({
          databaseAttachments: [{
            attachmentId: 'attachment_0123456789abcdef0123456789abcdef',
            databaseId: 'database_0123456789abcdef0123456789abcdef',
            displayName: 'Express Production',
            bindingName: 'primary',
            password: 'mock-password-must-not-render',
            databaseUrl: 'postgresql://must-not-render',
          }],
        })}
        busy={false}
        {...actions()}
      />,
    )
    expect(screen.getByText('Express Production')).toBeTruthy()
    expect(screen.getByText('Ready · Primary binding · Backend connection managed')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Add MiniBase Database' })).toBeNull()
    expect(view.container.textContent).not.toContain('mock-password-must-not-render')
    expect(view.container.textContent).not.toContain('postgresql://')
  })

  test('clearly rejects static Vite database attachment', () => {
    render(
      <DeploymentCard
        deployment={deployment({ strategy: 'vite-static', containerPort: 80 })}
        busy={false}
        {...actions()}
      />,
    )
    expect(screen.getByText('Database attachment is unavailable for this deployment strategy.')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Add MiniBase Database' })).toBeNull()
  })
})
