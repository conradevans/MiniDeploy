import { useCallback, useEffect, useState } from 'react'

import {
  deleteApplication,
  deployApplication,
  getDeployLogs,
  getDeployments,
  getHistory,
  getRuntimeLogs,
  redeployApplication,
  restartApplication,
  rollbackApplication,
} from './api'

import DeployForm from './components/DeployForm'
import DeploymentCard from './components/DeploymentCard'
import HistoryList from './components/HistoryList'
import Modal from './components/Modal'

import './App.css'

export default function App() {
  const [deployments, setDeployments] = useState([])
  const [loading, setLoading] = useState(true)
  const [busyApp, setBusyApp] = useState('')
  const [deploying, setDeploying] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const [modal, setModal] = useState({
    title: '',
    type: '',
    content: '',
    versions: [],
  })

  const loadDeployments = useCallback(async () => {
    try {
      const result = await getDeployments()
      setDeployments(Array.isArray(result) ? result : [])
      setError('')
    } catch (err) {
      setError(`Failed to load deployments: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadDeployments()
  }, [loadDeployments])

  async function handleDeploy(config) {
    setDeploying(true)
    setNotice('')
    setError('')

    try {
      const result = await deployApplication(config)

      setNotice(
        `${result.app} deployed successfully at ${result.port}.`,
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

  async function performAction(
    app,
    action,
    successMessage,
  ) {
    setBusyApp(app)
    setNotice('')
    setError('')

    try {
      await action(app)
      setNotice(successMessage)
      await loadDeployments()
    } catch (err) {
      setError(err.message)
    } finally {
      setBusyApp('')
    }
  }

  async function showLogs(app, deployLogs = false) {
    try {
      const result = deployLogs
        ? await getDeployLogs(app)
        : await getRuntimeLogs(app)

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
      const result = await getHistory(app)

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
      deleteApplication,
      `${app} deleted.`,
    )
  }

  function handleRollback(app) {
    if (!window.confirm(`Rollback ${app} to its previous deployment?`)) {
      return
    }

    performAction(
      app,
      rollbackApplication,
      `${app} rolled back successfully.`,
    )
  }

  const runningCount = deployments.filter(
    (deployment) => deployment.status === 'running',
  ).length

  return (
    <>
      <main className="app-shell">
        <header className="topbar">
          <div className="brand">
            <div className="brand-mark">M</div>

            <div>
              <h1>MiniDeploy</h1>
              <p>ReactorLab deployment control plane</p>
            </div>
          </div>

          <div className="server-state">
            <span className="status-dot live" />
            mini-server online
          </div>
        </header>

        <section className="hero">
          <div>
            <p className="eyebrow">REACTORLAB / MINIDEPLOY</p>
            <h2>
              Deploy, monitor, and recover your applications.
            </h2>
            <p className="hero-copy">
              Git-driven deployments running on your private infrastructure,
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
                  busy={busyApp === deployment.app}
                  onLogs={(app) => showLogs(app, false)}
                  onDeployLogs={(app) => showLogs(app, true)}
                  onRestart={(app) =>
                    performAction(
                      app,
                      restartApplication,
                      `${app} restarted.`,
                    )
                  }
                  onRedeploy={(app) =>
                    performAction(
                      app,
                      redeployApplication,
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
          MiniDeploy · ReactorLab · Private infrastructure
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
