import { useEffect, useMemo, useRef, useState } from 'react'

import { safeMiniBaseDatabases } from '../utils/minibase'

function suggestedName(app) {
  const words = String(app)
    .split('-')
    .filter(Boolean)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
  return `${words.join(' ') || 'Application'} Production`
}

export default function DatabaseDialog({ api, deployment, onClose, onAttached }) {
  const [mode, setMode] = useState('create')
  const [displayName, setDisplayName] = useState(() => suggestedName(deployment.app))
  const [databaseID, setDatabaseID] = useState('')
  const [databases, setDatabases] = useState([])
  const [loading, setLoading] = useState(true)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const firstInput = useRef(null)

  useEffect(() => {
    let active = true
    api.getMiniBaseDatabases()
      .then((result) => {
        if (!active) return
        const safe = safeMiniBaseDatabases(result)
        setDatabases(safe)
        setDatabaseID(safe[0]?.id || '')
        setError('')
      })
      .catch(() => {
        if (active) setError('MiniBase is unavailable.')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => { active = false }
  }, [api])

  const canSubmit = useMemo(() => {
    if (pending) return false
    return mode === 'create' ? displayName.trim() !== '' : databaseID !== ''
  }, [databaseID, displayName, mode, pending])

  async function submit(event) {
    event.preventDefault()
    if (!canSubmit) return
    setPending(true)
    setError('')
    try {
      const input = mode === 'create'
        ? { mode, displayName: displayName.trim() }
        : { mode: 'attach', databaseId: databaseID }
      await api.attachMiniBaseDatabase(deployment.app, input)
      await onAttached()
    } catch {
      setError('MiniBase could not attach the database. The existing deployment remains available.')
      setPending(false)
    }
  }

  return (
    <form className="database-dialog" onSubmit={submit}>
      <p>Attach one primary PostgreSQL database. ReactorLab manages the backend database connection.</p>
      <fieldset disabled={pending}>
        <legend>Database source</legend>
        <label>
          <input
            ref={firstInput}
            type="radio"
            name="database-mode"
            value="create"
            checked={mode === 'create'}
            onChange={() => setMode('create')}
            autoFocus
          />
          Create new
        </label>
        <label>
          <input
            type="radio"
            name="database-mode"
            value="attach"
            checked={mode === 'attach'}
            onChange={() => setMode('attach')}
          />
          Attach existing
        </label>
      </fieldset>

      {mode === 'create' ? (
        <label className="field">
          <span>Display name</span>
          <input value={displayName} onChange={(event) => setDisplayName(event.target.value)} disabled={pending} />
        </label>
      ) : loading ? (
        <div className="empty-state compact">Loading available databases…</div>
      ) : databases.length === 0 ? (
        <div className="empty-state compact">No ready unattached MiniBase databases are available.</div>
      ) : (
        <label className="field">
          <span>Ready database</span>
          <select value={databaseID} onChange={(event) => setDatabaseID(event.target.value)} disabled={pending}>
            {databases.map((database) => (
              <option key={database.id} value={database.id}>{database.displayName} · Ready</option>
            ))}
          </select>
        </label>
      )}

      {error ? <div className="notice error">{error}</div> : null}
      <div className="dialog-actions">
        <button className="button secondary" type="button" onClick={onClose} disabled={pending}>Cancel</button>
        <button className="button primary" type="submit" disabled={!canSubmit}>
          {pending ? 'Attaching database…' : mode === 'create' ? 'Create and attach' : 'Attach database'}
        </button>
      </div>
    </form>
  )
}
