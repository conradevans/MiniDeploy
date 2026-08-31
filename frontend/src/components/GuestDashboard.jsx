import { useCallback, useEffect, useMemo, useState } from 'react'

import { createGuestApi } from '../api/guest'
import Brand from './Brand'
import GuestDeploymentCard from './GuestDeploymentCard'

export default function GuestDashboard({ api: providedApi = null }) {
  const api = useMemo(
    () => providedApi || createGuestApi(),
    [providedApi],
  )
  const [applications, setApplications] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const loadApplications = useCallback(async () => {
    setLoading(true)

    try {
      const result = await api.getDeployments()
      setApplications(Array.isArray(result) ? result : [])
      setError('')
    } catch (err) {
      setError(`Unable to load public applications: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }, [api])

  useEffect(() => {
    const timeout = window.setTimeout(loadApplications, 0)

    return () => window.clearTimeout(timeout)
  }, [loadApplications])

  const runningCount = applications.filter(
    (application) => application.status === 'running',
  ).length

  return (
    <main className="guest-page">
      <div className="site-shell">
        <header className="public-nav guest-nav">
          <Brand subtitle="Public platform view" />

          <div className="public-nav-actions">
            <a className="nav-link" href="/">
              About MiniDeploy
            </a>
            <a className="button secondary" href="/admin/">
              Admin Sign In
            </a>
          </div>
        </header>

        <section className="guest-hero">
          <div>
            <span className="guest-mode-badge">
              <span aria-hidden="true">◇</span>
              Read-only Guest Mode
            </span>
            <h1>Explore what MiniDeploy is running.</h1>
            <p>
              This public view shows the live applications published by the
              platform. Operational details and management capabilities remain
              available only to an authenticated administrator.
            </p>
          </div>

          <div className="guest-summary" aria-label="Application summary">
            <div>
              <span>APPLICATIONS</span>
              <strong>{applications.length}</strong>
            </div>
            <div>
              <span>RUNNING</span>
              <strong>{runningCount}</strong>
            </div>
          </div>
        </section>

        <section className="guest-applications">
          <div className="guest-section-heading">
            <div>
              <p className="eyebrow">PUBLIC APPLICATIONS</p>
              <h2>Live deployments</h2>
            </div>

            <button
              className="button secondary"
              type="button"
              onClick={loadApplications}
              disabled={loading}
            >
              {loading ? 'Refreshing…' : 'Refresh status'}
            </button>
          </div>

          {error ? (
            <div className="notice error">{error}</div>
          ) : loading && applications.length === 0 ? (
            <div className="empty-state">Loading public applications…</div>
          ) : applications.length === 0 ? (
            <div className="empty-state">
              No public applications are available right now.
            </div>
          ) : (
            <div className="guest-application-grid">
              {applications.map((application) => (
                <GuestDeploymentCard
                  application={application}
                  key={application.app}
                />
              ))}
            </div>
          )}
        </section>

        <aside className="guest-boundary-note">
          <span aria-hidden="true">i</span>
          <p>
            Guest Mode is intentionally limited to public availability data.
            Changes to applications require administrator authentication.
          </p>
        </aside>

        <footer className="public-footer">
          <span>MiniDeploy · Read-only platform view</span>
          <a href="/">Return to the project overview</a>
        </footer>
      </div>
    </main>
  )
}
