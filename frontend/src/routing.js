import {
  ADMIN_API_MODE_LEGACY,
  ADMIN_API_MODE_PUBLIC,
} from './api/admin'

export const FRONTEND_MODE_PRIVATE_ADMIN = 'private-admin'
export const FRONTEND_MODE_PUBLIC = 'public'

export function readFrontendMode(documentObject = document) {
  const configuredMode = documentObject
    .querySelector('meta[name="minideploy-mode"]')
    ?.getAttribute('content')

  return configuredMode === FRONTEND_MODE_PRIVATE_ADMIN
    ? FRONTEND_MODE_PRIVATE_ADMIN
    : FRONTEND_MODE_PUBLIC
}

export function resolveFrontendRoute(mode, pathname) {
  if (mode === FRONTEND_MODE_PRIVATE_ADMIN) {
    return {
      screen: 'admin',
      adminApiMode: ADMIN_API_MODE_LEGACY,
    }
  }

  if (pathname === '/admin' || pathname.startsWith('/admin/')) {
    return {
      screen: 'admin',
      adminApiMode: ADMIN_API_MODE_PUBLIC,
    }
  }

  if (pathname === '/guest' || pathname.startsWith('/guest/')) {
    return {
      screen: 'guest',
      adminApiMode: null,
    }
  }

  return {
    screen: 'home',
    adminApiMode: null,
  }
}
