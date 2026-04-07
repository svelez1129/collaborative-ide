import { useEffect, useRef } from 'react'
import { EditorView, keymap } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { defaultKeymap, indentWithTab } from '@codemirror/commands'
import { oneDark } from '@codemirror/theme-one-dark'
import { go } from '@codemirror/lang-go'
import { yCollab, yUndoManagerKeymap } from 'y-codemirror.next'

export default function EditorPane({ ydoc, awareness, undoManager, role }) {
  const containerRef = useRef(null)
  const viewRef = useRef(null)

  useEffect(() => {
    if (!containerRef.current || !ydoc || !awareness || !undoManager) return

    const ytext = ydoc.getText('content')
    const editable = role === 'editor'

    const state = EditorState.create({
      doc: ytext.toString(),
      extensions: [
        keymap.of([...yUndoManagerKeymap, ...defaultKeymap, indentWithTab]),
        oneDark,
        go(),
        yCollab(ytext, awareness, { undoManager }),
        EditorView.editable.of(editable),
        EditorView.theme({
          '&': { height: '100%', background: '#0e0e10' },
          '.cm-scroller': { fontFamily: "'JetBrains Mono', monospace", fontSize: '13px', lineHeight: '1.65' },
          '.cm-content': { caretColor: '#00adb5' },
          '.cm-cursor': { borderLeftColor: '#00adb5' },
          '.cm-gutters': { background: '#0e0e10', borderRight: '1px solid #2e2e38', color: '#4a4a5a' },
          '.cm-activeLineGutter': { background: '#ffffff08' },
          '.cm-activeLine': { background: '#ffffff08' },
          '.cm-selectionBackground': { background: '#00adb530 !important' },
          '&.cm-focused .cm-selectionBackground': { background: '#00adb540 !important' },
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
  }, [ydoc, awareness, undoManager, role])

  return <div className="editor-wrap" ref={containerRef} />
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
