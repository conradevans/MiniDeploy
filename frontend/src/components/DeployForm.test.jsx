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
        name: 'Show advanced Dockerfile settings',
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
})
