import { useEffect, useState } from 'react'

import { safeMiniBaseDatabases } from '../utils/minibase'

export default function DeployForm({
  onDeploy,
  busy,
  getMiniBaseDatabases,
}) {
  const [repoUrl, setRepoUrl] = useState('')
  const [containerPort, setContainerPort] = useState('')
  const [healthPath, setHealthPath] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [environmentRows, setEnvironmentRows] = useState([])
  const [databaseId, setDatabaseId] = useState('')
  const [databases, setDatabases] = useState([])
  const [databasesLoading, setDatabasesLoading] = useState(
    typeof getMiniBaseDatabases === 'function',
  )
  const [databaseError, setDatabaseError] = useState('')

  useEffect(() => {
    if (typeof getMiniBaseDatabases !== 'function') {
      return undefined
    }

    let active = true
    getMiniBaseDatabases()
      .then((result) => {
        if (!active) return
        setDatabases(safeMiniBaseDatabases(result))
        setDatabaseError('')
      })
      .catch(() => {
        if (active) {
          setDatabaseError(
            'MiniBase is unavailable. You can still deploy without a database.',
          )
        }
      })
      .finally(() => {
        if (active) setDatabasesLoading(false)
      })

    return () => {
      active = false
    }
  }, [getMiniBaseDatabases])

  function addEnvironmentRow() {
    setEnvironmentRows((rows) => [
      ...rows,
      {
        name: '',
        value: '',
      },
    ])
  }

  function updateEnvironmentRow(index, field, value) {
    setEnvironmentRows((rows) =>
      rows.map((row, rowIndex) =>
        rowIndex === index
          ? {
            ...row,
            [field]: value,
          }
          : row,
      ),
    )
  }

  function removeEnvironmentRow(index) {
    setEnvironmentRows((rows) =>
      rows.filter((_, rowIndex) => rowIndex !== index),
    )
  }

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
      const trimmedPort = containerPort.trim()
      const trimmedHealthPath = healthPath.trim()

      if (trimmedPort) {
        config.containerPort = Number(trimmedPort)
      }

      if (trimmedHealthPath) {
        config.healthPath = trimmedHealthPath
      }

      if (environmentRows.length > 0) {
        config.environment = Object.fromEntries(
          environmentRows.map((row) => [row.name.trim(), row.value]),
        )
      }

      if (databaseId) {
        config.databaseId = databaseId
      }
    }

    const success = await onDeploy(config)

    if (success) {
      setRepoUrl('')
      setContainerPort('')
      setHealthPath('')
      setEnvironmentRows([])
      setDatabaseId('')
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
        {showAdvanced ? 'Hide' : 'Show'} advanced deployment settings
      </button>

      {showAdvanced && (
        <div className="advanced-settings">
          <div className="advanced-grid">
            <label className="field">
              <span>Container port</span>
              <input
                type="number"
                min="1"
                max="65535"
                value={containerPort}
                onChange={(event) => setContainerPort(event.target.value)}
                placeholder="Automatic"
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

          <div className="database-selector">
            <label className="field" htmlFor="initial-minibase-database">
              <span>MiniBase database (optional)</span>
              <select
                id="initial-minibase-database"
                value={databaseId}
                onChange={(event) => setDatabaseId(event.target.value)}
                disabled={busy || databasesLoading}
              >
                <option value="">No database</option>
                {databases.map((database) => (
                  <option key={database.id} value={database.id}>
                    {database.displayName} · Ready
                  </option>
                ))}
              </select>
            </label>
            <small>
              {databasesLoading
                ? 'Loading ready unattached databases…'
                : databaseError ||
                  'Only existing ready unattached databases can be selected.'}
            </small>
          </div>

          <div className="environment-editor">
            <div className="environment-heading">
              <div>
                <strong>Runtime environment variables</strong>
                <p>
                  Values such as MONGODB_URI are stored securely and are
                  never shown again after deployment.
                </p>
              </div>

              <button
                className="button secondary compact"
                type="button"
                onClick={addEnvironmentRow}
                disabled={busy}
              >
                Add variable
              </button>
            </div>

            {environmentRows.map((row, index) => (
              <div className="environment-row" key={index}>
                <label className="field">
                  <span>Environment variable name {index + 1}</span>
                  <input
                    type="text"
                    value={row.name}
                    onChange={(event) =>
                      updateEnvironmentRow(
                        index,
                        'name',
                        event.target.value,
                      )
                    }
                    placeholder="MONGODB_URI"
                    disabled={busy}
                    autoCapitalize="none"
                    spellCheck={false}
                    required
                  />
                </label>

                <label className="field">
                  <span>Environment variable value {index + 1}</span>
                  <input
                    type="password"
                    value={row.value}
                    onChange={(event) =>
                      updateEnvironmentRow(
                        index,
                        'value',
                        event.target.value,
                      )
                    }
                    placeholder="Sensitive value"
                    disabled={busy}
                    autoComplete="new-password"
                  />
                </label>

                <button
                  className="button secondary environment-remove"
                  type="button"
                  onClick={() => removeEnvironmentRow(index)}
                  disabled={busy}
                  aria-label={`Remove environment variable ${index + 1}`}
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
        </div>
      )}
    </form>
  )
}
