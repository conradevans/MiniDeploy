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
    guestVisible: true,
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
    guestVisible: false,
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
    updateGuestVisibility: vi.fn(async (_app, guestVisible) => ({
      guestVisible,
    })),
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

describe('MiniDeploy Guest View visibility', () => {
  test('renders every deployment and saves an accessible optimistic toggle', async () => {
    let finishUpdate
    const api = makeApi({
      updateGuestVisibility: vi.fn(
        () => new Promise((resolve) => {
          finishUpdate = resolve
        }),
      ),
    })
    window.history.replaceState({}, '', '/admin/visibility')
    render(<AdminDashboard api={api} />)

    expect(screen.getByRole('button', { name: 'Visibility' })).toBeTruthy()
    expect(await screen.findByText('alpha-app')).toBeTruthy()
    expect(screen.getByText('beta-app')).toBeTruthy()
    expect(screen.getByText('Visible in Guest View')).toBeTruthy()
    expect(screen.getByText('Hidden from Guest View')).toBeTruthy()

    const alphaSwitch = screen.getByRole('switch', {
      name: 'List alpha-app in Guest View',
    })
    const betaSwitch = screen.getByRole('switch', {
      name: 'List beta-app in Guest View',
    })
    expect(alphaSwitch.getAttribute('aria-checked')).toBe('true')
    expect(betaSwitch.getAttribute('aria-checked')).toBe('false')

    fireEvent.click(betaSwitch)
    expect(betaSwitch.getAttribute('aria-checked')).toBe('true')
    expect(betaSwitch.disabled).toBe(true)
    expect(screen.getByText('Saving…')).toBeTruthy()

    await act(async () => {
      finishUpdate({ guestVisible: true })
      await Promise.resolve()
    })

    expect(api.updateGuestVisibility).toHaveBeenCalledWith('beta-app', true)
    expect(betaSwitch.getAttribute('aria-checked')).toBe('true')
    expect(betaSwitch.disabled).toBe(false)
    expect(
      screen.getByText('beta-app is now visible in Guest View.'),
    ).toBeTruthy()
  })

  test('reverts the switch and shows a concise error when saving fails', async () => {
    const api = makeApi({
      updateGuestVisibility: vi.fn().mockRejectedValue(
        new Error('visibility request failed'),
      ),
    })
    window.history.replaceState({}, '', '/admin/visibility')
    render(<AdminDashboard api={api} />)

    const betaSwitch = await screen.findByRole('switch', {
      name: 'List beta-app in Guest View',
    })

    await act(async () => {
      fireEvent.click(betaSwitch)
      await Promise.resolve()
    })

    expect(betaSwitch.getAttribute('aria-checked')).toBe('false')
    expect(betaSwitch.disabled).toBe(false)
    expect(screen.getByText('Hidden from Guest View')).toBeTruthy()
    expect(
      screen.getByText(
        'Failed to update beta-app visibility: visibility request failed',
      ),
    ).toBeTruthy()
  })
})
