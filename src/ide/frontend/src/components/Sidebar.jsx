import { useState } from 'react'

const AVATAR_COLORS = ['#00adb5','#7c5cfc','#f0a500','#4ec97e','#f47c7c','#00d4aa']
function avatarColor(id) {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}

export default function Sidebar({ myRole, participants, proposals, onRoleChange, onAccept, onReject, ydoc, sendJSON }) {
  const [showProposeModal, setShowProposeModal] = useState(false)
  const [propText, setPropText] = useState('')

  function submitProposal() {
    if (!propText.trim()) return
    const id = crypto.randomUUID()
    sendJSON({
      type: 'proposal',
      action: 'add',
      id,
      replacement: propText.trim(),
      startIndex: 0,
      endIndex: 0,
    })
    setPropText('')
    setShowProposeModal(false)
  }

  return (
    <>
      <aside className="sidebar">
        <div className="sidebar-section">
          <div className="sidebar-label">Participants</div>
          {participants.map(p => (
            <div className="participant" key={p.id}>
              <div className="avatar" style={{ background: avatarColor(p.id) }}>
                {p.id[0].toUpperCase()}
              </div>
              <span className="participant-name">{p.id}{p.isMe ? ' (you)' : ''}</span>
              {myRole === 'editor' && !p.isMe ? (
                <select
                  className="role-select"
                  value={p.role}
                  onChange={e => onRoleChange(p.id, e.target.value)}
                >
                  <option value="editor">editor</option>
                  <option value="proposer">proposer</option>
                  <option value="viewer">viewer</option>
                </select>
              ) : (
                <span className={`participant-role pr-${p.role}`}>{p.role}</span>
              )}
            </div>
          ))}
        </div>

        <div className="sidebar-divider" />

        <div className="sidebar-section">
          <div className="sidebar-label">
            Proposals {proposals.length > 0 && <span style={{ color: 'var(--amber)' }}>({proposals.length})</span>}
          </div>
        </div>

        <div className="proposals-area">
          {proposals.length === 0
            ? <div className="empty-proposals">No pending proposals.</div>
            : proposals.map(p => (
                <div className="proposal-card" key={p.id}>
                  <div className="proposal-header">
                    <span className="participant-role pr-proposer" style={{ fontSize: 10 }}>proposer</span>
                    <span className="proposal-author">{p.author}</span>
                  </div>
                  <div className="proposal-diff">
                    <span className="diff-add">+ {p.replacement}</span>
                  </div>
                  {myRole === 'editor' && (
                    <div className="proposal-actions">
                      <button className="prop-btn prop-accept" onClick={() => onAccept(p)}>✓ Accept</button>
                      <button className="prop-btn prop-reject" onClick={() => onReject(p.id)}>✕ Reject</button>
                    </div>
                  )}
                </div>
              ))
          }
          {myRole === 'proposer' && (
            <button className="propose-btn" onClick={() => setShowProposeModal(true)}>
              + Submit proposal
            </button>
          )}
        </div>
      </aside>

      {showProposeModal && (
        <div className="modal-overlay" onClick={e => e.target === e.currentTarget && setShowProposeModal(false)}>
          <div className="propose-modal">
            <div className="modal-title" style={{ marginBottom: 4 }}>Submit a proposal</div>
            <div className="modal-sub" style={{ marginBottom: 14 }}>
              Describe the change you'd like to suggest. Editors will see it and can accept or reject.
            </div>
            <textarea
              placeholder="e.g. Add error handling to main()"
              value={propText}
              onChange={e => setPropText(e.target.value)}
              autoFocus
            />
            <div className="propose-modal-actions">
              <button className="cancel-btn" onClick={() => setShowProposeModal(false)}>Cancel</button>
              <button className="primary-btn" onClick={submitProposal} disabled={!propText.trim()}>
                Submit
              </button>
            </div>
            <button className="modal-close" onClick={() => setShowProposeModal(false)}>✕</button>
          </div>
        </div>
      )}
    </>
  )
}
