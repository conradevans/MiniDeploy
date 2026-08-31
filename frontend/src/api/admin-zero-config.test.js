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
})
