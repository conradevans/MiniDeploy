import { describe, expect, test, vi } from 'vitest'

import {
  ADMIN_API_MODE_LEGACY,
  ADMIN_API_MODE_PUBLIC,
  createAdminApi,
} from './admin'

describe.each([
  ['public', ADMIN_API_MODE_PUBLIC, '/api/admin/deploy'],
  ['private', ADMIN_API_MODE_LEGACY, '/deploy'],
])('%s zero-config Admin API', (_name, mode, path) => {
  test('omits manual port and health settings', async () => {
    const requester = vi.fn(async () => ({}))
    const api = createAdminApi(mode, requester)

    await api.deployApplication({
      repoUrl: 'https://github.com/example/vite-app.git',
    })

    expect(requester).toHaveBeenCalledWith(path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        repoUrl: 'https://github.com/example/vite-app.git',
      }),
    })
  })

  test('submits runtime environment without adding port defaults', async () => {
    const requester = vi.fn(async () => ({}))
    const api = createAdminApi(mode, requester)

    await api.deployApplication({
      repoUrl: 'https://github.com/example/express-app.git',
      environment: {
        MONGODB_URI: 'mongodb://example.invalid/app',
      },
    })

    expect(requester).toHaveBeenCalledWith(path, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        repoUrl: 'https://github.com/example/express-app.git',
        environment: {
          MONGODB_URI: 'mongodb://example.invalid/app',
        },
      }),
    })
  })

  test('uses explicit replace semantics only when redeploy env is supplied', async () => {
    const requester = vi.fn(async () => ({}))
    const api = createAdminApi(mode, requester)

    await api.redeployApplication('express app')
    await api.redeployApplication('express app', {
      environment: {
        JWT_SECRET: 'replacement',
      },
    })
    await api.redeployApplication('express app', {
      environment: {},
    })

    const redeployPath = path
      .replace(/\/deploy$/, '/deployments/express%20app/redeploy')

    expect(requester.mock.calls).toEqual([
      [redeployPath, { method: 'POST' }],
      [
        redeployPath,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            environment: {
              JWT_SECRET: 'replacement',
            },
          }),
        },
      ],
      [
        redeployPath,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            environment: {},
          }),
        },
      ],
    ])
  })
})
