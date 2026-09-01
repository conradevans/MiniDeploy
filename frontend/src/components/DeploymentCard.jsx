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

function strategyLabel(strategy) {
  return (
    {
      'fullstack-vite-node': 'Full-stack Vite + Node/Express',
      'vite-static': 'Vite static',
      'node-express': 'Node/Express',
      dockerfile: 'Dockerfile',
    }[strategy] || strategy || 'Unknown'
  )
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
  const environmentVariables = Array.isArray(
    deployment.environmentVariables,
  )
    ? deployment.environmentVariables
    : []
  const services = Array.isArray(deployment.services)
    ? deployment.services
    : []
  const isFullstack =
    deployment.strategy === 'fullstack-vite-node' && services.length > 0

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

      {isFullstack ? (
        <div className="service-summary">
          <div className="project-strategy">
            <span className="meta-label">PROJECT STRATEGY</span>
            <strong>{strategyLabel(deployment.strategy)}</strong>
          </div>

          <div className="service-list">
            {services.map((service) => (
              <section className="service-card" key={service.name}>
                <div className="service-heading">
                  <div>
                    <span className="meta-label">SERVICE</span>
                    <strong>
                      {service.name === 'frontend' ? 'Frontend' : 'Backend'}
                    </strong>
                  </div>
                  <span
                    className={`status-pill ${
                      service.status === 'running' ? 'live' : 'down'
                    }`}
                  >
                    {String(service.status || 'unknown').toUpperCase()}
                  </span>
                </div>

                <div className="metadata-grid service-metadata">
                  <div>
                    <span className="meta-label">TYPE</span>
                    <strong>{strategyLabel(service.strategy)}</strong>
                  </div>
                  <div>
                    <span className="meta-label">PATH</span>
                    <strong>{service.path}/</strong>
                  </div>
                  <div>
                    <span className="meta-label">HOST PORT</span>
                    <strong>{service.port}</strong>
                  </div>
                  <div>
                    <span className="meta-label">CONTAINER PORT</span>
                    <strong>{service.containerPort}</strong>
                  </div>
                  <div>
                    <span className="meta-label">HEALTH</span>
                    <strong>{service.healthPath}</strong>
                  </div>
                  <div>
                    <span className="meta-label">NPM MODE</span>
                    <strong>{service.packageInstallMode || '—'}</strong>
                  </div>
                  <div className="service-image">
                    <span className="meta-label">IMAGE</span>
                    <strong className="truncate" title={service.image}>
                      {service.image}
                    </strong>
                  </div>
                </div>
              </section>
            ))}
          </div>
        </div>
      ) : (
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
      )}

      {environmentVariables.length > 0 && (
        <div className="environment-summary">
          <span className="meta-label">
            {isFullstack ? 'BACKEND RUNTIME ENVIRONMENT' : 'RUNTIME ENVIRONMENT'}
          </span>
          <div>
            {environmentVariables.map((name) => (
              <code key={name}>{name}</code>
            ))}
          </div>
          <small>Values are stored securely and are not displayed.</small>
        </div>
      )}

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
