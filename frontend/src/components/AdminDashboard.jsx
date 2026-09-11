import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { createAdminApi } from '../api/admin'
import GlobalHeader from './GlobalHeader'
import DeployForm from './DeployForm'
import DeploymentCard from './DeploymentCard'
import HistoryList from './HistoryList'
import Modal from './Modal'


function adminViewFromPath(pathname) {
  if (pathname === '/admin/new' || pathname.endsWith('/admin/new')) {
    return 'new'
  }

  if (
    pathname === '/admin/deployments' ||
    pathname.endsWith('/admin/deployments')
  ) {
    return 'deployments'
  }

  return 'overview'
}

function strategyLabel(strategy) {
  return {
    'fullstack-vite-node': 'Full-stack Vite + Node',
    'vite-static': 'React + Vite',
    'node-express': 'Node + Express',
    dockerfile: 'Dockerfile',
  }[strategy] || strategy || 'Unknown'
}

export default function AdminDashboard({ apiMode, api: providedApi = null }) {
  const api = useMemo(
    () => providedApi || createAdminApi(apiMode),
    [apiMode, providedApi],
  )
  const [deployments, setDeployments] = useState([])
  const [session, setSession] = useState(null)
  const [loading, setLoading] = useState(true)
  const [operations, setOperations] = useState({})
  const [deploying, setDeploying] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [view, setView] = useState(() =>
    adminViewFromPath(window.location.pathname),
  )
  const [modal, setModal] = useState({
    title: '',
    type: '',
    content: '',
    versions: [],
  })
  const feedbackTimers = useRef(new Map())
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    const timers = feedbackTimers.current
    return () => {
      mounted.current = false
      for (const timer of timers.values()) {
        window.clearTimeout(timer)
      }
      timers.clear()
    }
  }, [])

  const loadDeployments = useCallback(async () => {
    try {
      const result = await api.getDeployments()
      if (!mounted.current) return
      setDeployments(Array.isArray(result) ? result : [])
      setError('')
    } catch (err) {
      if (!mounted.current) return
      setError(`Failed to load deployments: ${err.message}`)
    } finally {
      if (mounted.current) setLoading(false)
    }
  }, [api])

  useEffect(() => {
    const timeout = window.setTimeout(loadDeployments, 0)

    return () => window.clearTimeout(timeout)
  }, [loadDeployments])

  useEffect(() => {
    if (!api.supportsSession) {
      return undefined
    }

    const timeout = window.setTimeout(async () => {
      try {
        setSession(await api.getSession())
      } catch {
        setSession(null)
      }
    }, 0)

    return () => window.clearTimeout(timeout)
  }, [api])

  useEffect(() => {
    const handlePopState = () => {
      setView(adminViewFromPath(window.location.pathname))
    }

    window.addEventListener('popstate', handlePopState)
    return () => window.removeEventListener('popstate', handlePopState)
  }, [])

  function navigateView(nextView) {
    const href = {
      overview: '/admin',
      new: '/admin/new',
      deployments: '/admin/deployments',
    }[nextView]

    if (!href) return

    window.history.pushState({}, '', href)
    setView(nextView)
    document.documentElement.scrollTop = 0
    document.body.scrollTop = 0
  }

  async function handleDeploy(config) {
    setDeploying(true)
    setNotice('')
    setError('')

    try {
      const result = await api.deployApplication(config)
      const strategy = {
        'fullstack-vite-node': 'Full-stack Vite + Node/Express',
        'vite-static': 'React + Vite',
        'node-express': 'Node + Express',
      }[result.strategy] || 'Dockerfile'
      const destination = Array.isArray(result.services)
        ? `with ${result.services.length} managed services.`
        : `on host port ${result.port}.`

      setNotice(
        `${result.app} deployed successfully using ${strategy} ${destination}`,
      )

      await loadDeployments()
      return true
    } catch (err) {
      setError(`Deployment failed: ${err.message}`)
      return false
    } finally {
      setDeploying(false)
    }
  }

  function clearOperation(app) {
    setOperations((current) => {
      const next = { ...current }
      delete next[app]
      return next
    })
  }

  function showOperationFeedback(app, actionName, phase) {
    setOperations((current) => ({
      ...current,
      [app]: { action: actionName, phase },
    }))

    const existingTimer = feedbackTimers.current.get(app)
    if (existingTimer) window.clearTimeout(existingTimer)
    const timer = window.setTimeout(() => {
      feedbackTimers.current.delete(app)
      if (mounted.current) clearOperation(app)
    }, 3000)
    feedbackTimers.current.set(app, timer)
  }

  async function performAction(
    app,
    actionName,
    action,
    successMessage,
    { removeOnSuccess = false } = {},
  ) {
    const existingTimer = feedbackTimers.current.get(app)
    if (existingTimer) {
      window.clearTimeout(existingTimer)
      feedbackTimers.current.delete(app)
    }
    setOperations((current) => ({
      ...current,
      [app]: { action: actionName, phase: 'pending' },
    }))
    setNotice('')
    setError('')

    try {
      await action(app)
      if (!mounted.current) return
      setNotice(successMessage)
      await loadDeployments()
      if (!mounted.current) return
      if (removeOnSuccess) {
        clearOperation(app)
      } else {
        showOperationFeedback(app, actionName, 'success')
      }
    } catch (err) {
      if (!mounted.current) return
      setError(err.message)
      showOperationFeedback(app, actionName, 'failed')
    }
  }

  async function showLogs(app, deployLogs = false) {
    try {
      const result = deployLogs
        ? await api.getDeployLogs(app)
        : await api.getRuntimeLogs(app)

      setModal({
        title: `${app} ${deployLogs ? 'deployment logs' : 'runtime logs'}`,
        type: 'logs',
        content: result.logs || 'No logs available.',
        versions: [],
      })
    } catch (err) {
      setError(`Failed to retrieve logs: ${err.message}`)
    }
  }

  async function showHistory(app) {
    try {
      const result = await api.getHistory(app)

      setModal({
        title: `${app} deployment history`,
        type: 'history',
        content: '',
        versions: result.versions || [],
      })
    } catch (err) {
      setError(`Failed to retrieve history: ${err.message}`)
    }
  }

  function handleDelete(app) {
    if (!window.confirm(`Delete ${app}? This removes the deployment.`)) {
      return
    }

    performAction(
      app,
      'delete',
      api.deleteApplication,
      `${app} deleted.`,
      { removeOnSuccess: true },
    )
  }

  function handleRollback(app) {
    if (!window.confirm(`Rollback ${app} to its previous deployment?`)) {
      return
    }

    performAction(
      app,
      'rollback',
      api.rollbackApplication,
      `${app} rolled back successfully.`,
    )
  }

  const healthyCount = deployments.filter((deployment) =>
    ['running', 'healthy'].includes(deployment.status),
  ).length

  const attentionCount = Math.max(deployments.length - healthyCount, 0)

  const databaseLinkedCount = deployments.filter(
    (deployment) =>
      Array.isArray(deployment.databaseAttachments) &&
      deployment.databaseAttachments.length > 0,
  ).length

  const strategyCounts = deployments.reduce((counts, deployment) => {
    const strategy = deployment.strategy || 'unknown'
    counts[strategy] = (counts[strategy] || 0) + 1
    return counts
  }, {})

  return (
    <>
      <main className="app-shell admin-shell">
        <GlobalHeader
          mode="admin"
          sessionLabel={session?.email ? `Admin · ${session.email}` : 'Admin'}
        />

        <div className="control-layout">
          <aside className="control-sidebar">
            <nav aria-label="MiniDeploy navigation">
              <button
                type="button"
                className={
                  view === 'overview'
                    ? 'control-nav-item active'
                    : 'control-nav-item'
                }
                onClick={() => navigateView('overview')}
              >
                Overview
              </button>

              <button
                type="button"
                className={
                  view === 'new'
                    ? 'control-nav-item active'
                    : 'control-nav-item'
                }
                onClick={() => navigateView('new')}
              >
                New Deployment
              </button>

              <button
                type="button"
                className={
                  view === 'deployments'
                    ? 'control-nav-item active'
                    : 'control-nav-item'
                }
                onClick={() => navigateView('deployments')}
              >
                Deployments
              </button>
            </nav>

            <p className="control-sidebar-note">
              Git-driven deployments running on private ReactorLab
              infrastructure.
            </p>
          </aside>

          <div className="control-content">
            {view === 'overview' && (
              <section className="admin-view">
                <div className="control-heading">
                  <p className="eyebrow">ADMIN / OVERVIEW</p>
                  <h1>Applications, under control.</h1>
                  <p className="hero-copy">
                    A quick view of everything MiniDeploy is currently running
                    across ReactorLab.
                  </p>
                </div>

                <div className="control-summary-grid">
                  <article className="control-summary-card">
                    <span>DEPLOYMENTS</span>
                    <strong>{deployments.length}</strong>
                    <small>Total managed applications</small>
                  </article>

                  <article className="control-summary-card">
                    <span>HEALTHY</span>
                    <strong>{healthyCount}</strong>
                    <small>
                      {attentionCount === 0
                        ? 'All deployments operational'
                        : `${attentionCount} need attention`}
                    </small>
                  </article>

                  <article className="control-summary-card">
                    <span>DATABASE LINKED</span>
                    <strong>{databaseLinkedCount}</strong>
                    <small>Connected to managed MiniBase</small>
                  </article>
                </div>

                <div className="overview-grid">
                  <article className="overview-panel">
                    <div className="overview-panel-heading">
                      <div>
                        <p className="eyebrow">RUNTIMES</p>
                        <h2>Deployment types</h2>
                      </div>
                    </div>

                    <div className="runtime-list">
                      {Object.keys(strategyCounts).length === 0 ? (
                        <p className="overview-muted">
                          No runtime data available.
                        </p>
                      ) : (
                        Object.entries(strategyCounts)
                          .sort(([a], [b]) => a.localeCompare(b))
                          .map(([strategy, count]) => (
                            <div className="runtime-row" key={strategy}>
                              <span>{strategyLabel(strategy)}</span>
                              <strong>{count}</strong>
                            </div>
                          ))
                      )}
                    </div>
                  </article>

                  <article className="overview-panel">
                    <div className="overview-panel-heading">
                      <div>
                        <p className="eyebrow">PLATFORM</p>
                        <h2>Publishing</h2>
                      </div>
                    </div>

                    <div className="platform-facts">
                      <div>
                        <span>PUBLIC DOMAIN</span>
                        <strong>reactorlab.dev</strong>
                      </div>

                      <div>
                        <span>DATABASE PLATFORM</span>
                        <strong>MiniBase PostgreSQL</strong>
                      </div>

                      <div>
                        <span>ACCESS</span>
                        <strong>Cloudflare protected</strong>
                      </div>
                    </div>
                  </article>
                </div>

                <section className="overview-apps">
                  <div className="section-heading">
                    <div>
                      <p className="eyebrow">CURRENT STATE</p>
                      <h2>Applications</h2>
                    </div>

                    <button
                      className="button secondary"
                      type="button"
                      onClick={() => navigateView('deployments')}
                    >
                      Manage deployments
                    </button>
                  </div>

                  {loading ? (
                    <div className="empty-state compact">
                      Loading deployments…
                    </div>
                  ) : deployments.length === 0 ? (
                    <div className="empty-state compact">
                      No applications deployed yet.
                    </div>
                  ) : (
                    <div className="overview-app-list">
                      {deployments.slice(0, 6).map((deployment) => {
                        const healthy = ['running', 'healthy'].includes(
                          deployment.status,
                        )

                        return (
                          <div
                            className="overview-app-row"
                            key={deployment.app}
                          >
                            <div className="overview-app-name">
                              <span
                                className={
                                  healthy
                                    ? 'status-dot live'
                                    : 'status-dot down'
                                }
                              />
                              <div>
                                <strong>{deployment.app}</strong>
                                <small>
                                  {strategyLabel(deployment.strategy)}
                                </small>
                              </div>
                            </div>

                            <span
                              className={
                                healthy
                                  ? 'overview-status live'
                                  : 'overview-status down'
                              }
                            >
                              {deployment.status || 'unknown'}
                            </span>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </section>
              </section>
            )}

            {view === 'new' && (
              <section className="admin-view">
                <div className="control-heading">
                  <p className="eyebrow">ADMIN / NEW DEPLOYMENT</p>
                  <h1>Ship a new application.</h1>
                  <p className="hero-copy">
                    Connect a Git repository and let MiniDeploy detect,
                    configure, and publish the supported runtime.
                  </p>
                </div>

                <div className="deployment-guide-grid">
                  <article className="overview-panel">
                    <p className="eyebrow">AUTO-DETECTION</p>
                    <h2>Supported applications</h2>

                    <div className="supported-runtime-list">
                      <div>
                        <strong>Dockerfile</strong>
                        <span>Use an application-defined container build.</span>
                      </div>

                      <div>
                        <strong>React + Vite</strong>
                        <span>Build and serve a static frontend.</span>
                      </div>

                      <div>
                        <strong>Node + Express</strong>
                        <span>Run a managed backend service.</span>
                      </div>

                      <div>
                        <strong>Full-stack</strong>
                        <span>
                          Vite frontend with a Node/Express backend.
                        </span>
                      </div>
                    </div>
                  </article>

                  <article className="overview-panel">
                    <p className="eyebrow">MINIBASE</p>
                    <h2>Managed database</h2>

                    <p className="guide-copy">
                      Attach an available MiniBase PostgreSQL database during
                      deployment. MiniDeploy securely injects DATABASE_URL into
                      supported backend applications.
                    </p>

                    <div className="guide-rule">
                      Do not manually add DATABASE_URL when a managed database
                      is attached.
                    </div>
                  </article>
                </div>

                <DeployForm
                  onDeploy={handleDeploy}
                  busy={deploying}
                  getMiniBaseDatabases={api.getMiniBaseDatabases}
                />

                {(notice || error) && (
                  <div className={`notice ${error ? 'error' : 'success'}`}>
                    {error || notice}
                  </div>
                )}
              </section>
            )}

            {view === 'deployments' && (
              <section className="admin-view">
                <div className="control-heading deployments-heading">
                  <p className="eyebrow">ADMIN / DEPLOYMENTS</p>
                  <h1>Manage applications.</h1>
                  <p className="hero-copy">
                    Inspect runtime state, logs, deployment history, restarts,
                    redeployments, rollbacks, and removal.
                  </p>
                </div>

                <div className="deployment-inline-summary">
                  <div>
                    <span>TOTAL</span>
                    <strong>{deployments.length}</strong>
                  </div>

                  <div>
                    <span>HEALTHY</span>
                    <strong>{healthyCount}</strong>
                  </div>

                  <div>
                    <span>ATTENTION</span>
                    <strong>{attentionCount}</strong>
                  </div>
                </div>

                {(notice || error) && (
                  <div className={`notice ${error ? 'error' : 'success'}`}>
                    {error || notice}
                  </div>
                )}

                <section className="deployment-section">
                  <div className="section-heading">
                    <div>
                      <p className="eyebrow">APPLICATIONS</p>
                      <h2>Deployments</h2>
                    </div>

                    <button
                      className="button secondary"
                      type="button"
                      onClick={loadDeployments}
                      disabled={loading}
                    >
                      Refresh
                    </button>
                  </div>

                  {loading ? (
                    <div className="empty-state">
                      Loading deployments…
                    </div>
                  ) : deployments.length === 0 ? (
                    <div className="empty-state">
                      No applications deployed yet.
                    </div>
                  ) : (
                    <div className="deployment-list">
                      {deployments.map((deployment) => (
                        <DeploymentCard
                          key={deployment.app}
                          deployment={deployment}
                          operation={operations[deployment.app] || null}
                          onLogs={(app) => showLogs(app, false)}
                          onDeployLogs={(app) => showLogs(app, true)}
                          onRestart={(app) =>
                            performAction(
                              app,
                              'restart',
                              api.restartApplication,
                              `${app} restarted.`,
                            )
                          }
                          onRedeploy={(app) =>
                            performAction(
                              app,
                              'redeploy',
                              api.redeployApplication,
                              `${app} redeployed successfully.`,
                            )
                          }
                          onHistory={showHistory}
                          onRollback={handleRollback}
                          onDelete={handleDelete}
                        />
                      ))}
                    </div>
                  )}
                </section>
              </section>
            )}
          </div>
        </div>

        <footer>
          MiniDeploy · ReactorLab · Administrator control plane
        </footer>
      </main>

      <Modal
        title={modal.title}
        onClose={() =>
          setModal({
            title: '',
            type: '',
            content: '',
            versions: [],
          })
        }
      >
        {modal.type === 'history' ? (
          <HistoryList versions={modal.versions} />
        ) : (
          <pre className="log-viewer">{modal.content}</pre>
        )}
      </Modal>

    </>
  )
}
