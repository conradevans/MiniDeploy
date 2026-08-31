import { describe, expect, test } from 'vitest'

import {
  FRONTEND_MODE_PRIVATE_ADMIN,
  FRONTEND_MODE_PUBLIC,
  readFrontendMode,
  resolveFrontendRoute,
} from './routing'

describe('frontend routing', () => {
  test('always selects legacy admin for the private control plane', () => {
    expect(
      resolveFrontendRoute(FRONTEND_MODE_PRIVATE_ADMIN, '/'),
    ).toEqual({
      screen: 'admin',
      adminApiMode: 'legacy',
    })
  })

  test('selects public, guest, and protected admin experiences by path', () => {
    expect(resolveFrontendRoute(FRONTEND_MODE_PUBLIC, '/')).toEqual({
      screen: 'home',
      adminApiMode: null,
    })

    expect(resolveFrontendRoute(FRONTEND_MODE_PUBLIC, '/guest/')).toEqual({
      screen: 'guest',
      adminApiMode: null,
    })

    expect(resolveFrontendRoute(FRONTEND_MODE_PUBLIC, '/admin/')).toEqual({
      screen: 'admin',
      adminApiMode: 'public',
    })
  })

  test('reads an explicit server-provided mode instead of the hostname', () => {
    document.head.innerHTML = `
      <meta name="minideploy-mode" content="private-admin" />
    `

    expect(readFrontendMode(document)).toBe(FRONTEND_MODE_PRIVATE_ADMIN)
  })
})
