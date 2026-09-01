import { describe, expect, test, vi } from 'vitest'

import { ADMIN_API_MODE_LEGACY, ADMIN_API_MODE_PUBLIC, createAdminApi } from './admin'

describe('MiniBase Admin API routing', () => {
  test.each([
    [ADMIN_API_MODE_PUBLIC, '/api/admin'],
    [ADMIN_API_MODE_LEGACY, ''],
  ])('uses the established protected namespace for %s mode', async (mode, base) => {
    const requester = vi.fn().mockResolvedValue({})
    const api = createAdminApi(mode, requester)
    await api.getMiniBaseDatabases()
    await api.getDatabaseAttachment('demo app')
    await api.attachMiniBaseDatabase('demo app', { mode: 'create', displayName: 'Demo Production' })

    expect(requester).toHaveBeenNthCalledWith(1, `${base}/minibase/databases`)
    expect(requester).toHaveBeenNthCalledWith(2, `${base}/deployments/demo%20app/database`)
    expect(requester).toHaveBeenNthCalledWith(3, `${base}/deployments/demo%20app/database`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ mode: 'create', displayName: 'Demo Production' }),
    })
  })
})
