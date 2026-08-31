import Brand from './Brand'

const platformSteps = [
  ['01', 'Source', 'Git push received'],
  ['02', 'Build', 'Container image created'],
  ['03', 'Verify', 'Health checks passed'],
  ['04', 'Publish', 'Traffic switched safely'],
]

export default function PublicHome() {
  return (
    <main className="public-page">
      <div className="public-glow" aria-hidden="true" />

      <div className="site-shell">
        <header className="public-nav">
          <Brand subtitle="A ReactorLab project" />

          <div className="public-nav-actions">
            <span className="availability-badge">
              <span className="status-dot live" />
              Platform online
            </span>

            <a className="nav-link" href="/admin/">
              Admin Sign In
            </a>
          </div>
        </header>

        <section className="landing-hero">
          <div className="landing-copy">
            <p className="hero-kicker">
              SELF-HOSTED DEPLOYMENT PLATFORM
            </p>

            <h1 aria-label="Ship software on infrastructure you own.">
              Ship software on
              <span> infrastructure you own.</span>
            </h1>

            <p className="landing-summary">
              MiniDeploy turns Git repositories into healthy, routed
              applications—with zero-downtime releases, automatic recovery,
              and a control plane built from first principles.
            </p>

            <div className="landing-actions">
              <a className="button primary large" href="/guest/">
                Continue as Guest
                <span aria-hidden="true">→</span>
              </a>

              <a className="button secondary large" href="/admin/">
                Admin Sign In
              </a>
            </div>

            <p className="landing-note">
              Guest access is public and read-only. Administrator access is
              protected by Cloudflare Access.
            </p>
          </div>

          <div className="platform-preview" aria-label="Deployment pipeline">
            <div className="preview-toolbar">
              <span className="preview-label">LIVE DELIVERY PIPELINE</span>
              <span className="preview-pulse">READY</span>
            </div>

            <div className="pipeline-list">
              {platformSteps.map(([number, title, detail], index) => (
                <div className="pipeline-step" key={title}>
                  <span className="pipeline-number">{number}</span>

                  <span className="pipeline-copy">
                    <strong>{title}</strong>
                    <small>{detail}</small>
                  </span>

                  <span
                    className={`pipeline-state ${index === 3 ? 'active' : ''}`}
                  >
                    {index === 3 ? 'LIVE' : 'DONE'}
                  </span>
                </div>
              ))}
            </div>

            <div className="preview-route">
              <span>PUBLIC ROUTE</span>
              <strong>*.reactorlab.dev</strong>
            </div>
          </div>
        </section>

        <section className="capability-grid" aria-label="Platform capabilities">
          <article>
            <span className="capability-index">01</span>
            <h2>Safe releases</h2>
            <p>
              Candidate containers are built and checked before live traffic
              moves. Failed releases leave the healthy version untouched.
            </p>
          </article>

          <article>
            <span className="capability-index">02</span>
            <h2>Private by default</h2>
            <p>
              Workloads bind to loopback, public traffic enters through a
              secure tunnel, and administration stays behind a distinct trust
              boundary.
            </p>
          </article>

          <article>
            <span className="capability-index">03</span>
            <h2>Built to recover</h2>
            <p>
              Health validation, version history, zero-downtime rollback, and
              persistent activity records make recovery a first-class path.
            </p>
          </article>
        </section>

        <footer className="public-footer">
          <span>MiniDeploy · Designed and operated by ReactorLab</span>
          <span>Go · React · Docker · Caddy · Cloudflare</span>
        </footer>
      </div>
    </main>
  )
}
