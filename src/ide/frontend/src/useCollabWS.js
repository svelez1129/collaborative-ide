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

import (
\t"fmt"
\t"math"
)

func fibonacci(n int) int {
\tif n <= 1 {
\t\treturn n
\t}
\treturn fibonacci(n-1) + fibonacci(n-2)
}

func main() {
\tfmt.Println("GoCollab IDE — Hello!")
\tfmt.Println()
\tfmt.Println("Fibonacci sequence:")
\tfor i := 0; i < 10; i++ {
\t\tfmt.Printf("  fib(%d) = %d\\n", i, fibonacci(i))
\t}
\tfmt.Printf("\\nPi ≈ %.6f\\n", math.Pi)
\tfmt.Printf("sqrt(2) ≈ %.6f\\n", math.Sqrt(2))
}
`

/**
 * Custom WebSocket provider that bridges our Go backend with Yjs.
 *
 * Protocol (all messages over one WS connection):
 *   Binary frames  → full Yjs state snapshot (Y.encodeStateAsUpdate)
 *   Text frames    → JSON control message:
 *     { type: 'participants', list: [{id, role},...] }
 *     { type: 'proposal',    action: 'add'|'accept'|'reject', proposal: {...} }
 *     { type: 'role_change', userID, role }
 *     { type: 'run_result',  lines, status }
 *     { type: 'awareness',   data: base64 }
 *     { type: 'sync_sv',     data: base64, from: userID }  ← state vector from reconnecting client
 *     { type: 'sync_update', data: base64, to: userID }    ← diff response, routed to requester
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

    // After the doc state is settled (received from server or STARTER inserted),
    // broadcast our state vector so other clients can send us anything we missed.
    function doSyncHandshake() {
      const sv = Y.encodeStateVector(ydoc)
      ws.send(JSON.stringify({ type: 'sync_sv', data: toBase64(sv), from: userID }))
    }

    ws.onopen = () => {
      awareness.setLocalStateField('user', { name: userID, color: avatarColor || '#00adb5' })

      // If the server sends no existing doc within 150ms we are the session creator.
      setTimeout(() => {
        if (!receivedDoc && roleRef.current === 'editor') {
          const ytext = ydoc.getText('content')
          if (ytext.length === 0) {
            ytext.insert(0, STARTER)
            undoManager?.clear()
            onDocReady?.()
            doSyncHandshake()
          }
        }
      }, 150)
    }

    // Broadcast awareness changes (cursor / selection).
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
        // Full Yjs state snapshot from the server.
        if (!receivedDoc) {
          receivedDoc = true
          onDocReady?.()
        }
        Y.applyUpdate(ydoc, new Uint8Array(event.data), 'remote')
        // Tell other clients our state vector so they can fill any gaps.
        doSyncHandshake()
      } else {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'redirect') {
            // This node is not the Raft leader. Reconnect to the leader's port.
            const leaderPort = msg.port
            if (leaderPort && leaderPort !== location.port) {
              const proto = location.protocol === 'https:' ? 'wss' : 'ws'
              const leaderHost = location.hostname + ':' + leaderPort
              const newWS = new WebSocket(
                `${proto}://${leaderHost}/ws?user_id=${encodeURIComponent(userID)}&code=${encodeURIComponent(code)}`
              )
              ws.close()
              wsRef.current = newWS
            }
            return
          }
          if (msg.type === 'participants')   onParticipants?.(msg.list)
          else if (msg.type === 'proposal')  onProposal?.(msg)
          else if (msg.type === 'role_change') onRoleChange?.(msg)
          else if (msg.type === 'run_result')  onRunResult?.(msg)
          else if (msg.type === 'awareness')
            applyAwarenessUpdate(awareness, fromBase64(msg.data), 'remote')
          else if (msg.type === 'sync_sv') {
            // Another client wants to sync. Send them the diff they're missing.
            const theirSV  = fromBase64(msg.data)
            const diff     = Y.encodeStateAsUpdate(ydoc, theirSV)
            // Only reply if we have something new to offer.
            if (diff.length > 2) {
              ws.send(JSON.stringify({ type: 'sync_update', data: toBase64(diff), to: msg.from }))
            }
          }
          else if (msg.type === 'sync_update') {
            // The server routed a diff specifically for us.
            Y.applyUpdate(ydoc, fromBase64(msg.data), 'remote')
          }
        } catch {
          // ignore malformed
        }
      }
    }

    // When the local Yjs doc changes, send the FULL state (not just the
    // incremental update) so the server snapshot is always complete.
    const onUpdate = (update, origin) => {
      if (origin === 'remote') return
      if (roleRef.current === 'editor' && ws.readyState === WebSocket.OPEN) {
        ws.send(Y.encodeStateAsUpdate(ydoc))
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
