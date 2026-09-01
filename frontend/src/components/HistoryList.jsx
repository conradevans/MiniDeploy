function HistoryDetails({ version }) {
  const services = Array.isArray(version.services) ? version.services : []

  if (version.strategy === 'fullstack-vite-node' && services.length > 0) {
    return (
      <div className="history-details">
        <strong>Paired full-stack release</strong>
        <div className="history-meta">
          <span>
            {version.deployedAt
              ? new Date(version.deployedAt).toLocaleString()
              : 'Unknown time'}
          </span>
        </div>
        <div className="history-services">
          {services.map((service) => (
            <div key={service.name}>
              <strong>
                {service.name === 'frontend' ? 'Frontend' : 'Backend'}
              </strong>
              <span>{service.image}</span>
              <small>
                {service.path}/ · container port {service.containerPort} ·
                health {service.healthPath}
              </small>
            </div>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="history-details">
      <strong>{version.image}</strong>
      <div className="history-meta">
        <span>
          {version.deployedAt
            ? new Date(version.deployedAt).toLocaleString()
            : 'Unknown time'}
        </span>
        <span>Host port {version.port || '—'}</span>
        <span>Container port {version.containerPort || 'legacy'}</span>
        <span>Health {version.healthPath || 'legacy'}</span>
      </div>
    </div>
  )
}

export default function HistoryList({ versions }) {
  if (!versions?.length) {
    return (
      <div className="empty-state compact">
        No previous deployments available.
      </div>
    )
  }

  return (
    <div className="history-list">
      {versions.map((version, index) => (
        <div
          className="history-entry"
          key={`${version.image}-${version.deployedAt}-${index}`}
        >
          <div className="history-number">
            {index + 1}
          </div>

          <HistoryDetails version={version} />
        </div>
      ))}
    </div>
  )
}
