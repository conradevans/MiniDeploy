import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import AdminDashboard from './AdminDashboard'

const deployments = [
  {
    app: 'alpha-app',
    repoUrl: 'https://github.com/example/alpha-app.git',
    image: 'alpha:v1',
    port: 8001,
    containerPort: 3000,
    healthPath: '/health',
    strategy: 'node-express',
    status: 'running',
  },
  {
    app: 'beta-app',
    repoUrl: 'https://github.com/example/beta-app.git',
    image: 'beta:v1',
    port: 8002,
    containerPort: 3000,
    healthPath: '/health',
    strategy: 'node-express',
    status: 'running',
  },
]

function makeApi(overrides = {}) {
  return {
    supportsSession: false,
    getDeployments: vi.fn().mockResolvedValue(deployments),
    getMiniBaseDatabases: vi.fn().mockResolvedValue([]),
    deployApplication: vi.fn(),
    getRuntimeLogs: vi.fn(),
    getDeployLogs: vi.fn(),
    getHistory: vi.fn(),
    restartApplication: vi.fn().mockResolvedValue({}),
    redeployApplication: vi.fn().mockResolvedValue({}),
    rollbackApplication: vi.fn().mockResolvedValue({}),
    deleteApplication: vi.fn().mockResolvedValue({}),
    ...overrides,
  }
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('MiniDeploy operation feedback', () => {
  test('shows pending and Success in place, locks only the same app, then restores', async () => {
    let finishRedeploy
    const api = makeApi({
      redeployApplication: vi.fn(
        () => new Promise((resolve) => {
          finishRedeploy = resolve
        }),
      ),
    })
    window.history.replaceState({}, '', '/admin')
    render(<AdminDashboard api={api} />)
    fireEvent.click(screen.getByRole('button', { name: 'Deployments' }))

    const alpha = (await screen.findByRole('heading', {
      name: 'alpha-app',
    })).closest('article')
    const beta = screen.getByRole('heading', {
      name: 'beta-app',
    }).closest('article')

    vi.useFakeTimers()
    fireEvent.click(within(alpha).getByRole('button', { name: 'Redeploy' }))
    expect(
      within(alpha).getByRole('button', { name: 'Redeploying…' }).disabled,
    ).toBe(true)
    expect(within(alpha).getByRole('button', { name: 'Restart' }).disabled).toBe(
      true,
    )
    expect(within(beta).getByRole('button', { name: 'Redeploy' }).disabled).toBe(
      false,
    )

    await act(async () => {
      finishRedeploy({})
      await Promise.resolve()
    })
    expect(within(alpha).getByRole('button', { name: 'Success' })).toBeTruthy()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(within(alpha).getByRole('button', { name: 'Redeploy' })).toBeTruthy()
  })

  test('shows Failed in place and restores while retaining useful error detail', async () => {
    const api = makeApi({
      restartApplication: vi.fn().mockRejectedValue(
        new Error('restart request failed'),
      ),
    })
    window.history.replaceState({}, '', '/admin')
    render(<AdminDashboard api={api} />)
    fireEvent.click(screen.getByRole('button', { name: 'Deployments' }))
    const alpha = (await screen.findByRole('heading', {
      name: 'alpha-app',
    })).closest('article')

    vi.useFakeTimers()
    await act(async () => {
      fireEvent.click(within(alpha).getByRole('button', { name: 'Restart' }))
      await Promise.resolve()
    })
    expect(within(alpha).getByRole('button', { name: 'Failed' })).toBeTruthy()
    expect(screen.getByText('restart request failed')).toBeTruthy()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000)
    })
    expect(within(alpha).getByRole('button', { name: 'Restart' })).toBeTruthy()
  })
})
