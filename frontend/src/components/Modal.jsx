import { useEffect, useId } from 'react'

export default function Modal({
  title,
  children,
  onClose,
}) {
  const titleID = useId()

  useEffect(() => {
    if (!title) return undefined
    function closeOnEscape(event) {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [onClose, title])

  if (!title) {
    return null
  }

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose()
        }
      }}
    >
      <section className="modal" role="dialog" aria-modal="true" aria-labelledby={titleID}>
        <div className="modal-header">
          <h2 id={titleID}>{title}</h2>

          <button
            className="button secondary"
            onClick={onClose}
            type="button"
          >
            Close
          </button>
        </div>

        <div className="modal-content">
          {children}
        </div>
      </section>
    </div>
  )
}
