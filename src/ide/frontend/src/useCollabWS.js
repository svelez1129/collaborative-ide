import { useEffect, useRef, useCallback } from 'react'
import * as Y from 'yjs'
import {
  encodeAwarenessUpdate,
  applyAwarenessUpdate,
  removeAwarenessStates,
} from 'y-protocols/awareness'

function toBase64(bytes) {
  let s = ''
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
  return btoa(s)
}

function fromBase64(b64) {
  return Uint8Array.from(atob(b64), c => c.charCodeAt(0))
}

const STARTER = `package main

import "fmt"

func main() {
\tfmt.Println("Hello, World!")
}
`

/**
 * Custom WebSocket provider that bridges our Go backend with Yjs.
 *
 * Protocol (all messages over one WS connection):
 *   Binary frames  → raw Yjs update (Y.applyUpdate)
 *   Text frames    → JSON control message:
 *     { type: 'participants', list: [{id, role},...] }
 *     { type: 'proposal',    action: 'add'|'accept'|'reject', proposal: {...} }
 *     { type: 'role_change', userID, role }
 *     { type: 'run_result',  lines, status }
 */
export function useCollabWS({ ydoc, awareness, undoManager, code, userID, role, avatarColor, onParticipants, onProposal, onRoleChange, onRunResult, onDocReady }) {
  const wsRef = useRef(null)
  const roleRef = useRef(role)
  roleRef.current = role

  const send = useCallback((data) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(data)
    }
  }, [])

  useEffect(() => {
    let receivedDoc = false

    const ws = new WebSocket(
      `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?user_id=${encodeURIComponent(userID)}&code=${encodeURIComponent(code)}`
    )
    ws.binaryType = 'arraybuffer'
    wsRef.current = ws

    ws.onopen = () => {
      // Announce our cursor identity to other clients.
      awareness.setLocalStateField('user', { name: userID, color: avatarColor || '#00adb5' })

      // If we're an editor and the server sends no existing doc within 150ms,
      // we're the first user — seed the doc with starter code.
      setTimeout(() => {
        if (!receivedDoc && roleRef.current === 'editor') {
          const ytext = ydoc.getText('content')
          if (ytext.length === 0) {
            ytext.insert(0, STARTER)
            undoManager?.clear() // don't let users undo the initial content
            onDocReady?.()
          }
        }
      }, 150)
    }

    // Broadcast our awareness state whenever it changes (cursor move, selection).
    const onAwarenessChange = ({ added, updated, removed }) => {
      const changed = [...added, ...updated, ...removed]
      const update = encodeAwarenessUpdate(awareness, changed)
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'awareness', data: toBase64(update) }))
      }
    }
    awareness.on('change', onAwarenessChange)

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        if (!receivedDoc) {
          receivedDoc = true
          onDocReady?.()
        }
        const update = new Uint8Array(event.data)
        Y.applyUpdate(ydoc, update, 'remote')
      } else {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'participants') onParticipants?.(msg.list)
          else if (msg.type === 'proposal') onProposal?.(msg)
          else if (msg.type === 'role_change') onRoleChange?.(msg)
          else if (msg.type === 'run_result') onRunResult?.(msg)
          else if (msg.type === 'awareness') applyAwarenessUpdate(awareness, fromBase64(msg.data), 'remote')
        } catch {
          // ignore malformed
        }
      }
    }

    // When the local Yjs doc changes, send the binary update to the server.
    const onUpdate = (update, origin) => {
      if (origin === 'remote') return
      if (roleRef.current === 'editor' && ws.readyState === WebSocket.OPEN) {
        ws.send(update)
      }
    }
    ydoc.on('update', onUpdate)

    ws.onclose = () => { wsRef.current = null }

    return () => {
      awareness.off('change', onAwarenessChange)
      removeAwarenessStates(awareness, [ydoc.clientID], null)
      ydoc.off('update', onUpdate)
      ws.close()
    }
  }, [code, userID, ydoc, awareness])

  const sendJSON = useCallback((obj) => {
    send(JSON.stringify(obj))
  }, [send])

  return { sendJSON }
}
