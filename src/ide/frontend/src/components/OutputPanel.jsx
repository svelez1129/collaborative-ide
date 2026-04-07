// Matches Go compiler error lines: ./prog.go:10:5: some message
const GO_ERR_RE = /^(.*\.go):(\d+):(\d+):\s(.+)$/

// Keywords to highlight inside the error message
const ERR_KEYWORDS = [
  'undefined', 'undeclared', 'syntax error', 'unexpected',
  'cannot use', 'cannot convert', 'cannot assign',
  'declared and not used', 'imported and not used',
  'not enough arguments', 'too many arguments',
  'duplicate', 'invalid', 'mismatched', 'incompatible',
  'does not implement', 'no field or method',
]

const KEYWORD_RE = new RegExp(`(${ERR_KEYWORDS.map(k => k.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'gi')

function highlightErrMsg(msg) {
  const parts = msg.split(KEYWORD_RE)
  return parts.map((part, i) =>
    KEYWORD_RE.test(part)
      ? <span key={i} className="out-keyword">{part}</span>
      : part
  )
}

function OutputLine({ line }) {
  const { kind, text } = line

  if (kind === 'meta') {
    return <span className="output-line meta">{text}</span>
  }

  // Section header from Go compiler e.g. "# command-line-arguments"
  if (text.startsWith('#')) {
    return <span className="output-line out-section">{text}</span>
  }

  // Go compiler error: ./prog.go:10:5: message
  const match = kind === 'err' && text.match(GO_ERR_RE)
  if (match) {
    const [, file, ln, col, msg] = match
    return (
      <span className="output-line out-compiler-err">
        <span className="out-file">{file}</span>
        <span className="out-sep">:</span>
        <span className="out-linenum">{ln}</span>
        <span className="out-sep">:</span>
        <span className="out-colnum">{col}</span>
        <span className="out-sep">: </span>
        <span className="out-errmsg">{highlightErrMsg(msg)}</span>
      </span>
    )
  }

  // Panic / runtime error header
  if (text.startsWith('panic:') || text.startsWith('runtime error:')) {
    return <span className="output-line out-panic">{text}</span>
  }

  // Goroutine stack trace lines
  if (/^goroutine \d+/.test(text) || /^\s+\S+\.go:\d+/.test(text)) {
    return <span className="output-line out-trace">{text}</span>
  }

  return <span className={`output-line ${kind}`}>{text}</span>
}

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
          : lines.map((l, i) => <OutputLine key={i} line={l} />)
        }
      </div>
    </div>
  )
}
