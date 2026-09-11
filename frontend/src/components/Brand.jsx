export default function Brand({ href = '/' }) {
  return (
    <a
      aria-label="MiniDeploy home"
      className="brand brand-link"
      href={href}
    >
      <span className="brand-mark" aria-hidden="true">
        D
      </span>

      <strong className="brand-name">MiniDeploy</strong>
    </a>
  )
}
