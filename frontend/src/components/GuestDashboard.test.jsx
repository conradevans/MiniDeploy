import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import GuestDashboard from './GuestDashboard'

describe('Guest Mode', () => {
  test('renders only guest-safe data and no management controls', async () => {
    const api = {
      getDeployments: vi.fn(async () => [
        {
          app: 'portfolio-app',
          url: 'https://portfolio-app.reactorlab.dev',
          status: 'running',
          repoUrl: 'https://example.com/private.git',
          image: 'private-image:latest',
          container: 'private-container',
          port: 8123,
          containerPort: 3000,
          healthPath: '/private-health',
          logs: 'private output',
        },
      ]),
    }

    render(<GuestDashboard api={api} />)

    expect(
      await screen.findByRole('heading', { name: 'portfolio-app' }),
    ).toBeTruthy()
    expect(screen.getByText('Read-only Guest Mode')).toBeTruthy()
    expect(
      screen.getByRole('link', { name: /Open application/i }).href,
    ).toBe('https://portfolio-app.reactorlab.dev/')

    const forbiddenText = [
      'https://example.com/private.git',
      'private-image:latest',
      'private-container',
      '8123',
      '3000',
      '/private-health',
      'private output',
    ]

    for (const value of forbiddenText) {
      expect(screen.queryByText(value)).toBeNull()
    }

    for (const control of [
      'Deploy',
      'Logs',
      'Restart',
      'Redeploy',
      'Rollback',
      'Delete',
    ]) {
      expect(
        screen.queryByRole('button', { name: control }),
      ).toBeNull()
    }
  })
})
