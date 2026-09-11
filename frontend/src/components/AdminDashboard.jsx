import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { createAdminApi } from '../api/admin'
import GlobalHeader from './GlobalHeader'
import DeployForm from './DeployForm'
import DeploymentCard from './DeploymentCard'
import HistoryList from './HistoryList'
import Modal from './Modal'

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

  const runningCount = deployments.filter(
    (deployment) => deployment.status === 'running',
  ).length

  return (
    <>
      <main className="app-shell admin-shell">
        <GlobalHeader
          mode="admin"
          sessionLabel={session?.email ? `Admin · ${session.email}` : 'Admin'}
        />

        <section className="hero admin-hero">
          <div>
            <p className="eyebrow">REACTORLAB / MINIDEPLOY / ADMIN</p>
            <h1>Deploy, monitor, and recover your applications.</h1>
            <p className="hero-copy">
              Git-driven deployments running on private infrastructure and
              published securely through ReactorLab.
            </p>
          </div>

          <div className="stats">
            <div className="stat">
              <span>DEPLOYMENTS</span>
              <strong>{deployments.length}</strong>
            </div>

            <div className="stat">
              <span>RUNNING</span>
              <strong>{runningCount}</strong>
            </div>

            <div className="stat">
              <span>DOMAIN</span>
              <strong>reactorlab.dev</strong>
            </div>
          </div>
        </section>

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
            <div className="empty-state">Loading deployments…</div>
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
