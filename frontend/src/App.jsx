import AdminDashboard from './components/AdminDashboard'
import GuestDashboard from './components/GuestDashboard'
import PublicHome from './components/PublicHome'
import { resolveFrontendRoute } from './routing'

import './App.css'

export default function App({
  runtimeMode,
  pathname = window.location.pathname,
}) {
  const route = resolveFrontendRoute(runtimeMode, pathname)

  switch (route.screen) {
    case 'admin':
      return <AdminDashboard apiMode={route.adminApiMode} />
    case 'guest':
      return <GuestDashboard />
    default:
      return <PublicHome />
  }
}
