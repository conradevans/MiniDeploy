import { useState } from 'react'

export default function DeployForm({
  onDeploy,
  busy,
}) {
  const [repoUrl, setRepoUrl] = useState('')
  const [containerPort, setContainerPort] = useState('80')
  const [healthPath, setHealthPath] = useState('/')

  async function handleSubmit(event) {
    event.preventDefault()

    const trimmedRepo = repoUrl.trim()

    if (!trimmedRepo) {
      return
    }

    const port = Number(containerPort)

    const success = await onDeploy({
      repoUrl: trimmedRepo,
      containerPort: Number.isFinite(port) ? port : 80,
      healthPath: healthPath.trim() || '/',
    })

    if (success) {
      setRepoUrl('')
      setContainerPort('80')
      setHealthPath('/')
    }
  }

  return (
    <form className="deploy-panel" onSubmit={handleSubmit}>
      <div className="deploy-heading">
        <div>
          <p className="eyebrow">NEW DEPLOYMENT</p>
          <h2>Deploy a Git repository</h2>
        </div>

        <span className="secure-badge">
          PRIVATE CONTROL PLANE
        </span>
      </div>

      <div className="deploy-grid">
        <label className="field repo-field">
          <span>Git repository</span>
          <input
            type="text"
            value={repoUrl}
            onChange={(event) => setRepoUrl(event.target.value)}
            placeholder="https://github.com/user/project.git"
            disabled={busy}
          />
        </label>

        <label className="field">
          <span>Container port</span>
          <input
            type="number"
            min="1"
            max="65535"
            value={containerPort}
            onChange={(event) => setContainerPort(event.target.value)}
            disabled={busy}
          />
        </label>

        <label className="field">
          <span>Health path</span>
          <input
            type="text"
            value={healthPath}
            onChange={(event) => setHealthPath(event.target.value)}
            placeholder="/health"
            disabled={busy}
          />
        </label>

        <button
          className="button primary deploy-button"
          type="submit"
          disabled={busy || !repoUrl.trim()}
        >
          {busy ? 'Deploying…' : 'Deploy'}
        </button>
      </div>
    </form>
  )
}
