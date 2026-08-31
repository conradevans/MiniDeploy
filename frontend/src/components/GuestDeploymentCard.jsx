export default function GuestDeploymentCard({ application }) {
  const running = application.status === 'running'

  return (
    <article className="guest-deployment-card">
      <div className="guest-card-state">
        <span className={`status-dot ${running ? 'live' : 'down'}`} />
        <span className={`status-pill ${running ? 'live' : 'down'}`}>
          {String(application.status || 'unknown').toUpperCase()}
        </span>
      </div>

      <div className="guest-card-copy">
        <p>PUBLIC APPLICATION</p>
        <h2>{application.app}</h2>
        <a href={application.url} target="_blank" rel="noreferrer">
          {application.url}
        </a>
      </div>

      <a
        className="button secondary guest-open-button"
        href={application.url}
        target="_blank"
        rel="noreferrer"
      >
        Open application
        <span aria-hidden="true">↗</span>
      </a>
    </article>
  )
}
