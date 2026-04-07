import { useState, useMemo, useCallback } from 'react'
import * as Y from 'yjs'
import { useCollabWS } from '../useCollabWS'
import EditorPane from './EditorPane'
import OutputPanel from './OutputPanel'
import Sidebar from './Sidebar'
import ShareModal from './ShareModal'

export default function IDE({ session }) {
  const { code, userID, role: initialRole, avatarColor } = session

  const [myRole, setMyRole] = useState(initialRole)
  const [participants, setParticipants] = useState([{ id: userID, role: initialRole, isMe: true }])
  const [proposals, setProposals] = useState([])
  const [outputLines, setOutputLines] = useState([])
  const [outputStatus, setOutputStatus] = useState(null)
  const [running, setRunning] = useState(false)
  const [showShare, setShowShare] = useState(false)

  // Single Yjs document for the session
  const ydoc = useMemo(() => new Y.Doc(), [])

  const onParticipants = useCallback((list) => {
    setParticipants(list.map(p => ({ ...p, isMe: p.id === userID })))
  }, [userID])

  const onProposal = useCallback((msg) => {
    if (msg.action === 'add') {
      setProposals(prev => [...prev, msg.proposal])
    } else if (msg.action === 'accept' || msg.action === 'reject') {
      setProposals(prev => prev.filter(p => p.id !== msg.proposal.id))
    }
  }, [])

  const onRoleChange = useCallback((msg) => {
    if (msg.userID === userID) setMyRole(msg.role)
    setParticipants(prev => prev.map(p => p.id === msg.userID ? { ...p, role: msg.role } : p))
  }, [userID])

  const onRunResult = useCallback((msg) => {
    setOutputStatus(msg.status)
    setOutputLines(msg.lines)
    setRunning(false)
  }, [])

  const { sendJSON } = useCollabWS({ ydoc, code, userID, role: myRole, onParticipants, onProposal, onRoleChange, onRunResult })

  function handleRoleChange(targetID, newRole) {
    sendJSON({ type: 'role_change', userID: targetID, role: newRole })
    // Optimistic update
    setParticipants(prev => prev.map(p => p.id === targetID ? { ...p, role: newRole } : p))
  }

  function handleAcceptProposal(proposal) {
    const ytext = ydoc.getText('content')
    if (proposal.startIndex !== proposal.endIndex) {
      ytext.delete(proposal.startIndex, proposal.endIndex - proposal.startIndex)
    }
    ytext.insert(proposal.startIndex, proposal.replacement)
    sendJSON({ type: 'proposal', action: 'accept', proposal })
    setProposals(prev => prev.filter(p => p.id !== proposal.id))
  }

  function handleRejectProposal(id) {
    sendJSON({ type: 'proposal', action: 'reject', proposal: { id } })
    setProposals(prev => prev.filter(p => p.id !== id))
  }

  async function runCode() {
    if (running) return
    setRunning(true)
    setOutputStatus(null)
    setOutputLines([{ kind: 'meta', text: '▸ Compiling and running…' }])

    const ytext = ydoc.getText('content')
    try {
      const res = await fetch('/run', {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: ytext.toString(),
      })
      const data = await res.json()
      const lines = []
      if (data.Errors) {
        setOutputStatus('err')
        data.Errors.split('\n').filter(Boolean).forEach(l => lines.push({ kind: 'err', text: l }))
      } else {
        setOutputStatus('ok')
        const events = data.Events || []
        if (events.length === 0) {
          lines.push({ kind: 'meta', text: '▸ (no output)' })
        } else {
          events.forEach(ev => lines.push({ kind: ev.Kind === 'stderr' ? 'err' : 'ok', text: ev.Message }))
        }
      }
      setOutputLines(lines)
      sendJSON({ type: 'run_result', lines, status: data.Errors ? 'err' : 'ok' })
    } catch (e) {
      const lines = [{ kind: 'err', text: e.message }]
      setOutputStatus('err')
      setOutputLines(lines)
      sendJSON({ type: 'run_result', lines, status: 'err' })
    } finally {
      setRunning(false)
    }
  }

  const roleBadgeClass = `role-badge role-${myRole}`

  return (
    <div className="app">
      {/* Titlebar */}
      <header className="titlebar">
        <div className="logo">
          <div className="logo-icon">Go</div>GoCollab
        </div>
        <div className="titlebar-sep" />
        <div className="file-tab"><span className="dot" />main.go</div>
        <div className="titlebar-right">
          <div className="session-badge" onClick={() => setShowShare(true)}>
            <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="5" cy="8" r="2.5"/><circle cx="12" cy="4" r="2"/><circle cx="12" cy="12" r="2"/>
              <line x1="7.4" y1="7" x2="10.1" y2="5.3"/><line x1="7.4" y1="9" x2="10.1" y2="10.7"/>
            </svg>
            {code}
          </div>
          <div className={roleBadgeClass}>{myRole}</div>
          {myRole !== 'viewer' && (
            <button className="run-btn" onClick={runCode} disabled={running}>
              {running
                ? <><span className="spinner" /> Running…</>
                : <><svg viewBox="0 0 16 16" fill="currentColor" width="13" height="13"><path d="M4 2.5l9 5.5-9 5.5V2.5z"/></svg>Run</>
              }
            </button>
          )}
        </div>
      </header>

      {/* Sidebar */}
      <Sidebar
        myRole={myRole}
        participants={participants}
        proposals={proposals}
        onRoleChange={handleRoleChange}
        onAccept={handleAcceptProposal}
        onReject={handleRejectProposal}
        ydoc={ydoc}
        sendJSON={sendJSON}
      />

      {/* Main area: editor + output */}
      <div className="main-area">
        <EditorPane ydoc={ydoc} role={myRole} />
        <OutputPanel lines={outputLines} status={outputStatus} running={running} />
      </div>

      {/* Share modal */}
      {showShare && <ShareModal code={code} onClose={() => setShowShare(false)} />}
    </div>
  )
}
