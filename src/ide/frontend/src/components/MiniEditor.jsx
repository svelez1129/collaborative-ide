import { useEffect, useRef } from 'react'
import { EditorView, keymap } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { defaultKeymap, indentWithTab, history, historyKeymap } from '@codemirror/commands'
import { oneDark } from '@codemirror/theme-one-dark'
import { go } from '@codemirror/lang-go'

/**
 * Lightweight CodeMirror editor with no Yjs — used for the proposal modal.
 * Exposes the EditorView via viewRef so the parent can read the content.
 */
export default function MiniEditor({ initialContent, viewRef }) {
  const containerRef = useRef(null)

  useEffect(() => {
    if (!containerRef.current) return

    const state = EditorState.create({
      doc: initialContent ?? '',
      extensions: [
        keymap.of([...historyKeymap, ...defaultKeymap, indentWithTab]),
        history(),
        oneDark,
        go(),
        EditorView.theme({
          '&': { height: '100%', background: '#0e0e10' },
          '.cm-scroller': {
            fontFamily: "'JetBrains Mono', monospace",
            fontSize: '12px',
            lineHeight: '1.6',
            minHeight: '120px',
          },
          '.cm-content': { caretColor: '#00adb5' },
          '.cm-cursor': { borderLeftColor: '#00adb5' },
          '.cm-gutters': { background: '#0e0e10', borderRight: '1px solid #2e2e38', color: '#4a4a5a' },
          '.cm-activeLine': { background: '#ffffff08' },
          '.cm-selectionBackground': { background: '#00adb530 !important' },
        }),
      ],
    })

    const view = new EditorView({ state, parent: containerRef.current })
    if (viewRef) viewRef.current = view

    // Focus and move cursor to end
    view.focus()
    view.dispatch({ selection: { anchor: state.doc.length } })

    return () => {
      view.destroy()
      if (viewRef) viewRef.current = null
    }
  }, [initialContent])

  return (
    <div
      ref={containerRef}
      style={{
        border: '1px solid var(--line)',
        borderRadius: 'var(--radius)',
        overflow: 'hidden',
        marginBottom: 12,
      }}
    />
  )
}
