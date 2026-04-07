import { useEffect, useRef, useCallback } from 'react'
import * as Y from 'yjs'

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
 *   Binary frames  → raw Yjs update (Y.applyUpdate)
 *   Text frames    → JSON control message:
 *     { type: 'participants', list: [{id, role},...] }
 *     { type: 'proposal',    action: 'add'|'accept'|'reject', proposal: {...} }
 *     { type: 'role_change', userID, role }
 *     { type: 'run_result',  lines, status }
 */
export function useCollabWS({ ydoc, code, userID, role, onParticipants, onProposal, onRoleChange, onRunResult }) {
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
      // If we're an editor and the server sends no existing doc within 150ms,
      // we're the first user — seed the doc with starter code.
      setTimeout(() => {
        if (!receivedDoc && roleRef.current === 'editor') {
          const ytext = ydoc.getText('content')
          if (ytext.length === 0) {
            ytext.insert(0, STARTER)
          }
        }
      }, 150)
    }

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        receivedDoc = true
        const update = new Uint8Array(event.data)
        Y.applyUpdate(ydoc, update, 'remote')
      } else {
        try {
          const msg = JSON.parse(event.data)
          if (msg.type === 'participants') onParticipants?.(msg.list)
          else if (msg.type === 'proposal') onProposal?.(msg)
          else if (msg.type === 'role_change') onRoleChange?.(msg)
          else if (msg.type === 'run_result') onRunResult?.(msg)
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
      ydoc.off('update', onUpdate)
      ws.close()
    }
  }, [code, userID, ydoc])

  const sendJSON = useCallback((obj) => {
    send(JSON.stringify(obj))
  }, [send])

  return { sendJSON }
}
