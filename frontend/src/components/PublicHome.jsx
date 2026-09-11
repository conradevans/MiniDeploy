import GlobalHeader from './GlobalHeader'

export default function PublicHome() {
  return (
    <main className="public-page">
      <div className="site-shell">
        <GlobalHeader mode="root" />

        <section className="landing-hero">
          <div className="landing-copy">
            <p className="hero-kicker">MINIDEPLOY</p>
            <h1>Run supported application repositories on your own server.</h1>
            <p className="landing-summary">
              MiniDeploy turns supported repositories into self-hosted
              applications on the Dell, using a consistent deployment and
              routing process instead of repeated manual container work.
            </p>

            <dl className="access-explanation">
              <div>
                <dt>What it does</dt>
                <dd>Builds and runs supported applications as managed deployments.</dd>
              </div>
              <div>
                <dt>How it works</dt>
                <dd>Uses MiniDeploy's existing deployment strategies, manages runtime state, publishes routes through ReactorLab infrastructure, and provides supported restart, redeploy, and rollback operations.</dd>
              </div>
              <div>
                <dt>Why it is useful</dt>
                <dd>Removes repetitive server and container setup. Supported MiniBase applications can also receive a managed database connection without manually handling database credentials.</dd>
              </div>
            </dl>

            <section
              className="supported-strategies"
              aria-labelledby="supported-strategies-title"
            >
              <h2 id="supported-strategies-title">Currently supported</h2>
              <div className="supported-strategy-grid">
                <article>
                  <h3>Dockerfile</h3>
                  <p>Run applications that provide a working deployment container definition.</p>
                </article>
                <article>
                  <h3>Vite</h3>
                  <p>Static Vite frontends can be detected and built automatically.</p>
                </article>
                <article>
                  <h3>Node + Express</h3>
                  <p>Conventional Node.js and Express services can be detected and deployed automatically.</p>
                </article>
                <article>
                  <h3>Vite + Node/Express</h3>
                  <p>Supported full-stack repositories can deploy frontend and backend together.</p>
                </article>
              </div>
              <p className="supported-strategy-note">
                Automatic detection targets these Vite and conventional
                Node/Express shapes. A repository Dockerfile is the escape
                hatch for other stacks; MiniBase attachment currently targets
                the Node/Express backend paths.
              </p>
            </section>
          </div>

          <aside className="access-card">
            <p className="eyebrow">CHOOSE ACCESS</p>
            <h2>Open MiniDeploy</h2>
            <p>
              Administrator access opens deployment and lifecycle controls.
              Guest access provides a restricted, read-only view of public
              application availability.
            </p>
            <div className="access-actions">
              <a className="button primary" href="/admin/">
                Open Administrator
              </a>
              <a className="button secondary" href="/guest/">
                Guest Overview
              </a>
            </div>
          </aside>
        </section>

        <footer className="public-footer">
          <span>MiniDeploy · ReactorLab deployment control plane</span>
          <span>Self-hosted on the Dell</span>
        </footer>
      </div>
    </main>
  )
}
