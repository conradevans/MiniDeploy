import { describe, expect, test, vi } from 'vitest'

import { createGuestApi } from './guest'

describe('guest API', () => {
  test('offers only the guest deployment read endpoint', async () => {
    const requester = vi.fn(async () => [])
    const api = createGuestApi(requester)

    expect(Object.keys(api)).toEqual(['getDeployments'])

    await api.getDeployments()

    expect(requester).toHaveBeenCalledOnce()
    expect(requester).toHaveBeenCalledWith('/api/guest/deployments')
  })
})
