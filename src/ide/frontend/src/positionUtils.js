import * as Y from 'yjs'

export function toBase64(bytes) {
  let s = ''
  for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
  return btoa(s)
}

export function fromBase64(b64) {
  return Uint8Array.from(atob(b64), c => c.charCodeAt(0))
}

/**
 * Encode a [from, to] range in a Y.Text as base64 relative positions.
 * Relative positions stay valid even as other edits happen around them.
 */
export function encodeRange(ytext, from, to) {
  const relStart = Y.createRelativePositionFromTypeIndex(ytext, from)
  const relEnd   = Y.createRelativePositionFromTypeIndex(ytext, to)
  return {
    relStart: toBase64(Y.encodeRelativePosition(relStart)),
    relEnd:   toBase64(Y.encodeRelativePosition(relEnd)),
  }
}

/**
 * Resolve encoded relative positions back to absolute indices.
 * Returns null if either position has been deleted from the document.
 */
export function resolveRange(ydoc, relStartB64, relEndB64) {
  try {
    const relStart = Y.decodeRelativePosition(fromBase64(relStartB64))
    const relEnd   = Y.decodeRelativePosition(fromBase64(relEndB64))
    const absStart = Y.createAbsolutePositionFromRelativePosition(relStart, ydoc)
    const absEnd   = Y.createAbsolutePositionFromRelativePosition(relEnd, ydoc)
    if (!absStart || !absEnd) return null
    return { start: absStart.index, end: absEnd.index }
  } catch {
    return null
  }
}

/**
 * Returns true if the proposal's target range has been modified since submission.
 */
export function isStale(proposal, ydoc) {
  if (!proposal.relStart || !proposal.relEnd) return false
  const range = resolveRange(ydoc, proposal.relStart, proposal.relEnd)
  if (!range) return true  // positions were deleted
  if (!proposal.originalText) return false  // insertion — never stale
  const current = ydoc.getText('content').toString().slice(range.start, range.end)
  return current !== proposal.originalText
}
