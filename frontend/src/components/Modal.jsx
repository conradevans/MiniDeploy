export default function Modal({
  title,
  children,
  onClose,
}) {
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
      <section className="modal">
        <div className="modal-header">
          <h2>{title}</h2>

          <button
            className="button secondary"
            onClick={onClose}
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
