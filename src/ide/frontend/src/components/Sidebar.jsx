import { useState, useEffect, useRef } from 'react'
import { encodeRange, isStale } from '../positionUtils'
import MiniEditor from './MiniEditor'

const AVATAR_COLORS = ['#00adb5','#7c5cfc','#f0a500','#4ec97e','#f47c7c','#00d4aa']
function avatarColor(id) {
  let h = 0
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}

export default function Sidebar({ myRole, participants, proposals, onRoleChange, onAccept, onReject, ydoc, sendJSON, editorViewRef }) {
  const [showProposeModal, setShowProposeModal] = useState(false)
  const [selection, setSelection]   = useState(null) // { from, to, text }
  const [staleIds, setStaleIds]     = useState(new Set())
  const miniEditorViewRef           = useRef(null)

  // Live-check staleness whenever the doc or proposal list changes.
  useEffect(() => {
    if (!ydoc) return
    function check() {
      setStaleIds(new Set(proposals.filter(p => isStale(p, ydoc)).map(p => p.id)))
    }
    check()
    ydoc.on('update', check)
    return () => ydoc.off('update', check)
  }, [proposals, ydoc])

  function openProposeModal() {
    // Snapshot the current editor selection.
    const view = editorViewRef?.current
    if (view) {
      const { from, to } = view.state.selection.main
      const text = view.state.doc.sliceString(from, to)
      const startLine = view.state.doc.lineAt(from).number
      const endLine = view.state.doc.lineAt(to).number
      setSelection({ from, to, text, startLine, endLine })
    } else {
      setSelection(null)
    }
    setShowProposeModal(true)
  }

  function submitProposal() {
    const miniView = miniEditorViewRef.current
    if (!miniView || !selection) return

    const replacement = miniView.state.doc.toString()
    // Don't submit if nothing changed
    if (replacement === selection.text) return

    const id    = crypto.randomUUID()
    const ytext = ydoc.getText('content')
    const { relStart, relEnd } = encodeRange(ytext, selection.from, selection.to)

    sendJSON({
      type: 'proposal',
      action: 'add',
      id,
      replacement,
      originalText: selection.text,
      relStart,
      relEnd,
      startLine: selection.startLine,
      endLine: selection.endLine,
    })
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
            : proposals.map(p => {
                const stale = staleIds.has(p.id)
                return (
                  <div className={`proposal-card${stale ? ' proposal-stale' : ''}`} key={p.id}>
                    <div className="proposal-header">
                      <span className="participant-role pr-proposer" style={{ fontSize: 10 }}>proposer</span>
                      <span className="proposal-author">{p.author}</span>
                      {p.startLine && (
                        <span className="proposal-lines">
                          L{p.startLine}{p.endLine !== p.startLine ? `–${p.endLine}` : ''}
                        </span>
                      )}
                      {stale && <span className="stale-badge">stale</span>}
                    </div>
                    <div className="proposal-diff">
                      {p.originalText
                        ? <span className="diff-del">- {p.originalText}</span>
                        : null
                      }
                      <span className="diff-add">+ {p.replacement}</span>
                    </div>
                    {stale && (
                      <div className="stale-note">Target was modified — accept will be rejected.</div>
                    )}
                    {myRole === 'editor' && (
                      <div className="proposal-actions">
                        <button className="prop-btn prop-accept" onClick={() => onAccept(p)}>✓ Accept</button>
                        <button className="prop-btn prop-reject" onClick={() => onReject(p.id)}>✕ Reject</button>
                      </div>
                    )}
                  </div>
                )
              })
          }
          {(myRole === 'proposer' || myRole === 'editor') && (
            <button className="propose-btn" onClick={openProposeModal}>
              + Submit proposal
            </button>
          )}
        </div>
      </aside>

      {showProposeModal && (
        <div className="modal-overlay" onClick={e => e.target === e.currentTarget && setShowProposeModal(false)}>
          <div className="propose-modal">
            <div className="modal-title" style={{ marginBottom: 4 }}>Propose a change</div>
            {selection && selection.from !== selection.to ? (
              <>
                <div className="modal-sub" style={{ marginBottom: 12 }}>
                  Edit the selected code below. The diff will be shown to editors.
                </div>
                <MiniEditor
                  initialContent={selection.text}
                  viewRef={miniEditorViewRef}
                />
                <div className="propose-modal-actions">
                  <button className="cancel-btn" onClick={() => setShowProposeModal(false)}>Cancel</button>
                  <button className="primary-btn" onClick={submitProposal}>Submit</button>
                </div>
              </>
            ) : (
              <>
                <div className="modal-sub" style={{ marginBottom: 16 }}>
                  Select the code you want to change in the editor, then click Submit proposal.
                </div>
                <button className="cancel-btn" style={{ width: '100%' }} onClick={() => setShowProposeModal(false)}>
                  Close
                </button>
              </>
            )}
            <button className="modal-close" onClick={() => setShowProposeModal(false)}>✕</button>
          </div>
        </div>
      )}
    </>
  )
}
