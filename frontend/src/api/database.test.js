import { describe, expect, test, vi } from 'vitest'

import {
  ADMIN_API_MODE_LEGACY,
  ADMIN_API_MODE_PUBLIC,
  createAdminApi,
} from './admin'

describe('MiniBase Admin API routing', () => {
  test.each([
    [ADMIN_API_MODE_PUBLIC, '/api/admin'],
    [ADMIN_API_MODE_LEGACY, ''],
  ])(
    'keeps database discovery for new deployments in %s mode',
    async (mode, base) => {
      const requester = vi.fn().mockResolvedValue([])
      const api = createAdminApi(mode, requester)

      await api.getMiniBaseDatabases()

      expect(requester).toHaveBeenCalledTimes(1)
      expect(requester).toHaveBeenCalledWith(
        `${base}/minibase/databases`,
      )
    },
  )
})
