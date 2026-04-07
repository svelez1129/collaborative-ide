import { useState } from 'react'

export default function ShareModal({ code, onClose }) {
  const [copied, setCopied] = useState(false)

  function copyCode() {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    })
  }

  return (
    <div className="modal-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
      <div className="modal">
        <div className="modal-title">Share Session</div>
        <div className="modal-sub">Invite collaborators with this code.</div>
        <div className="invite-code">{code}</div>
        <button className="copy-btn" onClick={copyCode}>
          {copied ? 'Copied!' : 'Copy Invite Code'}
        </button>
        <button className="modal-close" onClick={onClose}>✕</button>
      </div>
    </div>
  )
}
