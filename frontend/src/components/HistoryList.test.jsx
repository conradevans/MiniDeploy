import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import HistoryList from './HistoryList'

afterEach(cleanup)

describe('HistoryList', () => {
  test('renders full-stack history as one paired release', () => {
    render(
      <HistoryList
        versions={[
          {
            image: 'minideploy-project-frontend:v1',
            strategy: 'fullstack-vite-node',
            deployedAt: '2026-09-01T00:00:00Z',
            services: [
              {
                name: 'frontend',
                path: 'frontend',
                image: 'minideploy-project-frontend:v1',
                containerPort: 80,
                healthPath: '/',
              },
              {
                name: 'backend',
                path: 'backend',
                image: 'minideploy-project-backend:v1',
                containerPort: 3000,
                healthPath: '/health',
              },
            ],
          },
        ]}
      />,
    )

    expect(screen.getByText('Paired full-stack release')).not.toBeNull()
    expect(screen.getByText('Frontend')).not.toBeNull()
    expect(screen.getByText('Backend')).not.toBeNull()
    expect(
      screen.getByText('minideploy-project-frontend:v1'),
    ).not.toBeNull()
    expect(
      screen.getByText('minideploy-project-backend:v1'),
    ).not.toBeNull()
  })
})
