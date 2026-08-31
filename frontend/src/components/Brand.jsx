export default function Brand({ subtitle, href = '/' }) {
  return (
    <a
      aria-label="MiniDeploy home"
      className="brand brand-link"
      href={href}
    >
      <span className="brand-mark" aria-hidden="true">
        M
      </span>

      <span>
        <strong className="brand-name">MiniDeploy</strong>
        <span className="brand-subtitle">{subtitle}</span>
      </span>
    </a>
  )
}
