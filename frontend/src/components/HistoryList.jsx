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

          <div className="history-details">
            <strong>{version.image}</strong>

            <div className="history-meta">
              <span>
                {version.deployedAt
                  ? new Date(version.deployedAt).toLocaleString()
                  : 'Unknown time'}
              </span>

              <span>
                Host port {version.port || '—'}
              </span>

              <span>
                Container port {version.containerPort || 'legacy'}
              </span>

              <span>
                Health {version.healthPath || 'legacy'}
              </span>
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
