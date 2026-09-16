export default function VisibilityView({
  deployments,
  loading,
  pending,
  onToggle,
}) {
  const visibleCount = deployments.filter(
    (deployment) => deployment.guestVisible,
  ).length
  const hiddenCount = deployments.length - visibleCount

  return (
    <section className="admin-view">
      <div className="control-heading visibility-heading">
        <p className="eyebrow">ADMIN / VISIBILITY</p>
        <h1>Visibility</h1>
        <p className="hero-copy">
          Control which deployments are listed in Guest View. Hidden
          deployments remain online and can still be accessed directly using
          their application URL.
        </p>
      </div>

      <div
        className="deployment-inline-summary"
        aria-label="Guest View visibility summary"
      >
        <div>
          <span>TOTAL</span>
          <strong>{deployments.length}</strong>
        </div>
        <div>
          <span>VISIBLE</span>
          <strong>{visibleCount}</strong>
        </div>
        <div>
          <span>HIDDEN</span>
          <strong>{hiddenCount}</strong>
        </div>
      </div>

      {loading ? (
        <div className="empty-state">Loading deployments…</div>
      ) : deployments.length === 0 ? (
        <div className="empty-state">No applications deployed yet.</div>
      ) : (
        <div className="visibility-list">
          {deployments.map((deployment) => {
            const isPending = Boolean(pending[deployment.app])
            const isVisible = Boolean(deployment.guestVisible)

            return (
              <div className="visibility-row" key={deployment.app}>
                <div className="visibility-copy">
                  <strong>{deployment.app}</strong>
                  <span className={isVisible ? 'visible' : 'hidden'}>
                    {isPending
                      ? 'Saving…'
                      : isVisible
                        ? 'Visible in Guest View'
                        : 'Hidden from Guest View'}
                  </span>
                </div>

                <button
                  type="button"
                  role="switch"
                  aria-checked={isVisible}
                  aria-label={`List ${deployment.app} in Guest View`}
                  className={
                    isVisible
                      ? 'visibility-switch visible'
                      : 'visibility-switch hidden'
                  }
                  disabled={isPending}
                  onClick={() => onToggle(deployment.app, !isVisible)}
                >
                  <span className="visibility-switch-thumb" />
                </button>
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
