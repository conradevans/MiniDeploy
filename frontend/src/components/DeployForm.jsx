import { useState } from 'react'

export default function DeployForm({
  onDeploy,
  busy,
}) {
  const [repoUrl, setRepoUrl] = useState('')
  const [containerPort, setContainerPort] = useState('80')
  const [healthPath, setHealthPath] = useState('/')
  const [showAdvanced, setShowAdvanced] = useState(false)

  async function handleSubmit(event) {
    event.preventDefault()

    const trimmedRepo = repoUrl.trim()

    if (!trimmedRepo) {
      return
    }

    const config = {
      repoUrl: trimmedRepo,
    }

    if (showAdvanced) {
      const port = Number(containerPort)

      config.containerPort = Number.isFinite(port) ? port : 80
      config.healthPath = healthPath.trim() || '/'
    }

    const success = await onDeploy(config)

    if (success) {
      setRepoUrl('')
      setContainerPort('80')
      setHealthPath('/')
      setShowAdvanced(false)
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

      <p className="deploy-copy">
        Paste a repository URL. MiniDeploy will use its Dockerfile or detect a
        supported zero-config project automatically.
      </p>

      <div className="deploy-grid">
        <label className="field repo-field">
          <span>Git repository</span>
          <input
            type="url"
            value={repoUrl}
            onChange={(event) => setRepoUrl(event.target.value)}
            placeholder="https://github.com/user/project.git"
            disabled={busy}
            autoCapitalize="none"
            spellCheck={false}
            required
          />
        </label>

        <button
          className="button primary deploy-button"
          type="submit"
          disabled={busy || !repoUrl.trim()}
        >
          {busy ? 'Analyzing and deploying…' : 'Deploy'}
        </button>
      </div>

      <button
        className="advanced-toggle"
        type="button"
        onClick={() => setShowAdvanced((visible) => !visible)}
        disabled={busy}
        aria-expanded={showAdvanced}
      >
        {showAdvanced ? 'Hide' : 'Show'} advanced Dockerfile settings
      </button>

      {showAdvanced && (
        <div className="advanced-grid">
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
        </div>
      )}
    </form>
  )
}
