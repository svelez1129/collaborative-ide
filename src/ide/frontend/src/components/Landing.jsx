import { useState } from 'react'

const COLORS = ['#00adb5','#7c5cfc','#f0a500','#4ec97e','#f47c7c']
function randomColor() { return COLORS[Math.floor(Math.random() * COLORS.length)] }

export default function Landing({ onJoin }) {
  const [userID, setUserID] = useState('')
  const [joinCode, setJoinCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleCreate() {
    if (!userID.trim()) { setError('Enter a display name first.'); return }
    setError(''); setLoading(true)
    try {
      const res = await fetch('/create', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: userID.trim() }),
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      onJoin({ code: data.code, userID: userID.trim(), role: data.role, avatarColor: randomColor() })
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  async function handleJoin() {
    if (!userID.trim()) { setError('Enter a display name first.'); return }
    if (!joinCode.trim()) { setError('Enter an invite code.'); return }
    setError(''); setLoading(true)
    try {
      const res = await fetch('/join', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: joinCode.trim().toUpperCase(), user_id: userID.trim() }),
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      onJoin({ code: data.code, userID: userID.trim(), role: data.role, avatarColor: randomColor() })
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  function onKey(e) { if (e.key === 'Enter') handleJoin() }

  return (
    <div className="landing">
      <div className="landing-card">
        <div className="landing-logo">
          <div className="landing-logo-icon">Go</div>
          <span className="landing-logo-text">GoCollab IDE</span>
        </div>

        <label className="field-label">Your display name</label>
        <input
          className="field-input"
          placeholder="e.g. sergio"
          value={userID}
          onChange={e => setUserID(e.target.value)}
          onKeyDown={onKey}
          autoFocus
        />

        {error && <div className="error-msg">{error}</div>}

        <button className="primary-btn" onClick={handleCreate} disabled={loading}>
          {loading ? 'Creating…' : 'Create new session'}
        </button>

        <div className="divider">or join existing</div>

        <label className="field-label">Invite code</label>
        <input
          className="field-input"
          placeholder="ABC-1234"
          value={joinCode}
          onChange={e => setJoinCode(e.target.value)}
          onKeyDown={onKey}
          style={{ textTransform: 'uppercase', letterSpacing: '2px' }}
        />

        <button className="primary-btn" onClick={handleJoin} disabled={loading}>
          {loading ? 'Joining…' : 'Join session'}
        </button>
      </div>
    </div>
  )
}
