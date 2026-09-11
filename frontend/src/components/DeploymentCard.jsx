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
  operation,
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
	const databaseSupported =
		deployment.strategy === 'node-express' ||
		deployment.strategy === 'fullstack-vite-node'
	const databaseAttachments = Array.isArray(deployment.databaseAttachments)
		? deployment.databaseAttachments
		: []
	const database = databaseAttachments.find(
		(attachment) => attachment?.bindingName === 'primary' &&
			typeof attachment.displayName === 'string',
	)
  const busy = operation?.phase === 'pending'

  function actionPresentation(action, label, pendingLabel) {
    if (operation?.action !== action) {
      return { label, className: '' }
    }
    switch (operation.phase) {
      case 'pending':
        return { label: pendingLabel, className: 'operation-pending' }
      case 'success':
        return { label: 'Success', className: 'operation-success' }
      case 'failed':
        return { label: 'Failed', className: 'operation-failed' }
      default:
        return { label, className: '' }
    }
  }

  const restart = actionPresentation('restart', 'Restart', 'Restarting…')
  const redeploy = actionPresentation('redeploy', 'Redeploy', 'Redeploying…')
  const rollback = actionPresentation('rollback', 'Rollback', 'Rolling back…')
  const remove = actionPresentation('delete', 'Delete', 'Deleting…')

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


      <section className="database-summary" aria-label="MiniBase database">
        <div>
          <span className="meta-label">MINIBASE</span>

          {database ? (
            <>
              <strong>{database.displayName}</strong>
              <small>
                Ready · Primary binding · Managed in MiniBase
              </small>
            </>
          ) : deployment.databaseDetached ? (
            <>
              <strong>Database detached</strong>
              <small>
                Reconnect this deployment from MiniBase.
              </small>
            </>
          ) : (
            <>
              <strong>No database attached</strong>
              <small>
                {databaseSupported
                  ? 'Attach or manage a PostgreSQL database from MiniBase.'
                  : 'Database attachment is unavailable for this deployment strategy.'}
              </small>
            </>
          )}
        </div>
      </section>
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
          type="button"
          onClick={() => onLogs(deployment.app)}
        >
          Logs
        </button>

        <button
          className="button secondary"
          type="button"
          onClick={() => onDeployLogs(deployment.app)}
        >
          Deploy Logs
        </button>

        <button
          type="button"
          className={'button secondary operation-feedback ' + restart.className}
          onClick={() => onRestart(deployment.app)}
          disabled={busy || operation?.action === 'restart'}
        >
          {restart.label}
        </button>

        <button
          type="button"
          className={'button secondary operation-feedback ' + redeploy.className}
          onClick={() => onRedeploy(deployment.app)}
          disabled={busy || operation?.action === 'redeploy'}
        >
          {redeploy.label}
        </button>

        <button
          className="button secondary"
          type="button"
          onClick={() => onHistory(deployment.app)}
        >
          History
        </button>

        <button
          type="button"
          className={'button secondary operation-feedback ' + rollback.className}
          onClick={() => onRollback(deployment.app)}
          disabled={busy || operation?.action === 'rollback'}
        >
          {rollback.label}
        </button>

        <button
          type="button"
          className={'button danger operation-feedback ' + remove.className}
          onClick={() => onDelete(deployment.app)}
          disabled={busy || operation?.action === 'delete'}
        >
          {remove.label}
        </button>
      </div>
    </article>
  )
}
