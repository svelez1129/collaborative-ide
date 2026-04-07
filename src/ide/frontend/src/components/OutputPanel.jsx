export default function OutputPanel({ lines, status, running }) {
  return (
    <div className="output-panel">
      <div className="output-header">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" style={{ color: 'var(--subtle)' }}>
          <rect x="2" y="2" width="12" height="12" rx="2" />
          <path d="M5 8h6M5 5h4M5 11h3" />
        </svg>
        <span className="output-label">Output</span>
        <span className="output-status">
          {running && <span className="status-run">● running</span>}
          {!running && status === 'ok' && <span className="status-ok">✓ exited 0</span>}
          {!running && status === 'err' && <span className="status-err">✕ build failed</span>}
        </span>
      </div>
      <div className="output-body">
        {lines.length === 0
          ? <span className="output-line meta">▸ Press Run to execute your Go code.</span>
          : lines.map((l, i) => (
              <span key={i} className={`output-line ${l.kind}`}>{l.text}</span>
            ))
        }
      </div>
    </div>
  )
}
