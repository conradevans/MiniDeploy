import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import DeployForm from './DeployForm'

afterEach(cleanup)

describe('DeployForm', () => {
  test('submits only the repository URL for zero-config deployment', async () => {
    const onDeploy = vi.fn(async () => true)

    render(<DeployForm onDeploy={onDeploy} busy={false} />)

    expect(screen.queryByLabelText('Container port')).toBeNull()
    expect(screen.queryByLabelText('Health path')).toBeNull()

    fireEvent.change(screen.getByLabelText('Git repository'), {
      target: {
        value: '  https://github.com/example/vite-app.git  ',
      },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(onDeploy).toHaveBeenCalledWith({
        repoUrl: 'https://github.com/example/vite-app.git',
      })
    })
  })

  test('keeps manual Dockerfile settings behind the advanced control', async () => {
    const onDeploy = vi.fn(async () => false)

    render(<DeployForm onDeploy={onDeploy} busy={false} />)

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show advanced deployment settings',
      }),
    )

    fireEvent.change(screen.getByLabelText('Git repository'), {
      target: {
        value: 'https://github.com/example/docker-app.git',
      },
    })
    fireEvent.change(screen.getByLabelText('Container port'), {
      target: { value: '3000' },
    })
    fireEvent.change(screen.getByLabelText('Health path'), {
      target: { value: '/health' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(onDeploy).toHaveBeenCalledWith({
        repoUrl: 'https://github.com/example/docker-app.git',
        containerPort: 3000,
        healthPath: '/health',
      })
    })
  })

  test('adds, removes, masks, submits, and clears runtime variables', async () => {
    const onDeploy = vi.fn(async () => true)

    render(<DeployForm onDeploy={onDeploy} busy={false} />)

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show advanced deployment settings',
      }),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Add variable' }))
    fireEvent.click(screen.getByRole('button', { name: 'Add variable' }))

    const firstValue = screen.getByLabelText('Environment variable value 1')
    expect(firstValue.getAttribute('type')).toBe('password')

    fireEvent.change(screen.getByLabelText('Environment variable name 1'), {
      target: { value: 'MONGODB_URI' },
    })
    fireEvent.change(firstValue, {
      target: { value: 'mongodb://example.invalid/app' },
    })

    fireEvent.change(screen.getByLabelText('Environment variable name 2'), {
      target: { value: 'REMOVE_ME' },
    })
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Remove environment variable 2',
      }),
    )

    expect(screen.queryByLabelText('Environment variable name 2')).toBeNull()

    fireEvent.change(screen.getByLabelText('Git repository'), {
      target: {
        value: 'https://github.com/example/express-app.git',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(onDeploy).toHaveBeenCalledWith({
        repoUrl: 'https://github.com/example/express-app.git',
        environment: {
          MONGODB_URI: 'mongodb://example.invalid/app',
        },
      })
    })

    await waitFor(() => {
      expect(
        screen.getByRole('button', {
          name: 'Show advanced deployment settings',
        }),
      ).not.toBeNull()
    })

    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show advanced deployment settings',
      }),
    )

    expect(screen.queryByLabelText('Environment variable name 1')).toBeNull()
  })
  test('selects only an existing ready unattached MiniBase database', async () => {
    const onDeploy = vi.fn(async () => true)
    const getMiniBaseDatabases = vi.fn(async () => [
      {
        id: 'database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        displayName: 'Ready Scheduler',
        status: 'ready',
        attached: false,
      },
      {
        id: 'database_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
        displayName: 'Already attached',
        status: 'ready',
        attached: true,
      },
      {
        id: 'database_cccccccccccccccccccccccccccccccc',
        displayName: 'Not ready',
        status: 'provisioning',
        attached: false,
      },
    ])

    render(
      <DeployForm
        onDeploy={onDeploy}
        busy={false}
        getMiniBaseDatabases={getMiniBaseDatabases}
      />,
    )
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show advanced deployment settings',
      }),
    )

    const selector = await screen.findByLabelText(
      'MiniBase database (optional)',
    )
    await screen.findByText('Ready Scheduler · Ready')
    expect(screen.queryByText('Already attached · Ready')).toBeNull()
    expect(screen.queryByText('Not ready · Ready')).toBeNull()

    fireEvent.change(selector, {
      target: { value: 'database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' },
    })
    fireEvent.change(screen.getByLabelText('Git repository'), {
      target: {
        value: 'https://github.com/example/scheduler.git',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(onDeploy).toHaveBeenCalledWith({
        repoUrl: 'https://github.com/example/scheduler.git',
        databaseId: 'database_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      })
    })
  })

  test('keeps no-database deployment available when MiniBase is unavailable', async () => {
    const onDeploy = vi.fn(async () => true)
    render(
      <DeployForm
        onDeploy={onDeploy}
        busy={false}
        getMiniBaseDatabases={vi.fn(async () => {
          throw new Error('unavailable')
        })}
      />,
    )
    fireEvent.click(
      screen.getByRole('button', {
        name: 'Show advanced deployment settings',
      }),
    )

    await screen.findByText(
      'MiniBase is unavailable. You can still deploy without a database.',
    )
    const selector = screen.getByLabelText('MiniBase database (optional)')
    expect(selector.value).toBe('')
    expect(selector.disabled).toBe(false)

    fireEvent.change(screen.getByLabelText('Git repository'), {
      target: {
        value: 'https://github.com/example/no-database.git',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Deploy' }))

    await waitFor(() => {
      expect(onDeploy).toHaveBeenCalledWith({
        repoUrl: 'https://github.com/example/no-database.git',
      })
    })
  })
})
