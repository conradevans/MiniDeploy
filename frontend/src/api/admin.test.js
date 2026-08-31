import { describe, expect, test, vi } from 'vitest'

import {
  ADMIN_API_MODE_LEGACY,
  ADMIN_API_MODE_PUBLIC,
  createAdminApi,
} from './admin'

async function exerciseAdminApi(api) {
  await api.getSession()
  await api.getDeployments()
  await api.deployApplication({
    repoUrl: 'https://example.com/app.git',
    containerPort: 3000,
    healthPath: '/health',
  })
  await api.getRuntimeLogs('demo app')
  await api.getDeployLogs('demo app')
  await api.getHistory('demo app')
  await api.restartApplication('demo app')
  await api.redeployApplication('demo app')
  await api.rollbackApplication('demo app')
  await api.deleteApplication('demo app')
}

describe('public admin API', () => {
  test('uses only the protected /api/admin namespace', async () => {
    const requester = vi.fn(async () => ({}))
    const api = createAdminApi(ADMIN_API_MODE_PUBLIC, requester)

    await exerciseAdminApi(api)

    const paths = requester.mock.calls.map(([path]) => path)

    expect(paths).toEqual([
      '/api/admin/session',
      '/api/admin/deployments',
      '/api/admin/deploy',
      '/api/admin/deployments/demo%20app/logs',
      '/api/admin/deployments/demo%20app/deploy-logs',
      '/api/admin/deployments/demo%20app/history',
      '/api/admin/deployments/demo%20app/restart',
      '/api/admin/deployments/demo%20app/redeploy',
      '/api/admin/deployments/demo%20app/rollback',
      '/api/admin/deployments/demo%20app',
    ])

    expect(paths.every((path) => path.startsWith('/api/admin/'))).toBe(true)
  })
})

describe('private admin API', () => {
  test('retains every legacy port-9000 path', async () => {
    const requester = vi.fn(async () => ({}))
    const api = createAdminApi(ADMIN_API_MODE_LEGACY, requester)

    await exerciseAdminApi(api)

    expect(api.supportsSession).toBe(false)
    expect(requester.mock.calls.map(([path]) => path)).toEqual([
      '/deployments',
      '/deploy',
      '/deployments/demo%20app/logs',
      '/deployments/demo%20app/deploy-logs',
      '/deployments/demo%20app/history',
      '/deployments/demo%20app/restart',
      '/deployments/demo%20app/redeploy',
      '/deployments/demo%20app/rollback',
      '/deployments/demo%20app',
    ])
  })
})
