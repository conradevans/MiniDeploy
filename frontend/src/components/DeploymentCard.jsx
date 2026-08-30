function publicUrl(app) {
  const label = String(app)
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
    .replace(/-+$/g, '')

  return `https://${label}.reactorlab.dev`
}

export default function DeploymentCard({
  deployment,
  busy,
  onLogs,
  onDeployLogs,
  onRestart,
  onRedeploy,
  onHistory,
  onRollback,
  onDelete,
}) {
  const url = publicUrl(deployment.app)
  const running = deployment.status === 'running'

  return (
    <article className="deployment-card">
      <div className="deployment-header">
        <div className="deployment-title">
          <div className={`status-dot ${running ? 'live' : 'down'}`} />

          <div>
            <h3>{deployment.app}</h3>
            <a
              href={url}
              target="_blank"
              rel="noreferrer"
              className="public-url"
            >
              {url}
            </a>
          </div>
        </div>

        <span className={`status-pill ${running ? 'live' : 'down'}`}>
          {String(deployment.status || 'unknown').toUpperCase()}
        </span>
      </div>

      <div className="repo-row">
        {deployment.repoUrl}
      </div>

      <div className="metadata-grid">
        <div>
          <span className="meta-label">HOST PORT</span>
          <strong>{deployment.port}</strong>
        </div>

        <div>
          <span className="meta-label">CONTAINER PORT</span>
          <strong>{deployment.containerPort}</strong>
        </div>

        <div>
          <span className="meta-label">HEALTH</span>
          <strong>{deployment.healthPath}</strong>
        </div>

        <div>
          <span className="meta-label">IMAGE</span>
          <strong className="truncate" title={deployment.image}>
            {deployment.image}
          </strong>
        </div>
      </div>

      <div className="actions">
        <a
          className="button secondary"
          href={url}
          target="_blank"
          rel="noreferrer"
        >
          Open
        </a>

        <button
          className="button secondary"
          onClick={() => onLogs(deployment.app)}
          disabled={busy}
        >
          Logs
        </button>

        <button
          className="button secondary"
          onClick={() => onDeployLogs(deployment.app)}
          disabled={busy}
        >
          Deploy Logs
        </button>

        <button
          className="button secondary"
          onClick={() => onRestart(deployment.app)}
          disabled={busy}
        >
          Restart
        </button>

        <button
          className="button secondary"
          onClick={() => onRedeploy(deployment.app)}
          disabled={busy}
        >
          Redeploy
        </button>

        <button
          className="button secondary"
          onClick={() => onHistory(deployment.app)}
          disabled={busy}
        >
          History
        </button>

        <button
          className="button secondary"
          onClick={() => onRollback(deployment.app)}
          disabled={busy}
        >
          Rollback
        </button>

        <button
          className="button danger"
          onClick={() => onDelete(deployment.app)}
          disabled={busy}
        >
          Delete
        </button>
      </div>
    </article>
  )
}
