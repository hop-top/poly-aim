import type { Filter } from './types.ts'

const KNOWN_TAG_KEYS = new Set([
  'in',
  'out',
  'provider',
  'family',
  'tool_call',
  'reasoning',
  'open_weights',
  'structured_output',
  'temperature',
])

interface Token {
  val: string
  quoted: boolean
}

/** Tokenise splits the query into tokens, respecting double-quoted strings. */
function tokenise(q: string): Token[] {
  q = q.trim()
  if (!q) return []

  const tokens: Token[] = []
  let i = 0
  const n = q.length

  while (i < n) {
    // skip whitespace
    if (q[i] === ' ' || q[i] === '\t') {
      i++
      continue
    }

    if (q[i] === '"') {
      // quoted string
      let j = i + 1
      while (j < n && q[j] !== '"') j++
      if (j >= n) {
        throw new Error('aim: unterminated quoted string in query')
      }
      tokens.push({ val: q.slice(i + 1, j), quoted: true })
      i = j + 1
      continue
    }

    // unquoted token — read until whitespace or quote
    let j = i
    while (j < n && q[j] !== ' ' && q[j] !== '\t' && q[j] !== '"') j++
    const raw = q.slice(i, j)
    i = j

    if (raw === ':') {
      throw new Error('aim: bare colon in query')
    }

    tokens.push({ val: raw, quoted: false })
  }

  return tokens
}

/** parseBool accepts only "true" or "false". */
function parseBool(s: string): boolean {
  if (s === 'true') return true
  if (s === 'false') return false
  throw new Error(`aim: invalid bool value "${s}": must be "true" or "false"`)
}

/**
 * parseQuery parses a string query into a Filter.
 * Throws for unknown tag keys, invalid bool values, or bare colons.
 *
 * Syntax:
 *   key:value   — structured tag (see known keys)
 *   "..."       — quoted free-text; colons inside are literal
 *   bare token  — free-text appended to Filter.query
 */
export function parseQuery(q: string): Filter {
  const tokens = tokenise(q)
  const filter: Filter = {}
  const freeText: string[] = []

  for (const tok of tokens) {
    if (tok.quoted) {
      freeText.push(tok.val)
      continue
    }

    const colonIdx = tok.val.indexOf(':')
    if (colonIdx === -1) {
      freeText.push(tok.val)
      continue
    }

    const key = tok.val.slice(0, colonIdx)
    const val = tok.val.slice(colonIdx + 1)

    if (!key || !val) {
      throw new Error(
        `aim: malformed tag "${tok.val}": key and value must both be non-empty`,
      )
    }

    if (!KNOWN_TAG_KEYS.has(key)) {
      throw new Error(`aim: unknown tag key "${key}"`)
    }

    applyTag(filter, key, val)
  }

  if (freeText.length > 0) {
    filter.query = freeText.join(' ')
  }

  return filter
}

function applyTag(f: Filter, key: string, val: string): void {
  switch (key) {
    case 'in':
      f.input = [...(f.input ?? []), ...val.split(',')]
      break
    case 'out':
      f.output = [...(f.output ?? []), ...val.split(',')]
      break
    case 'provider':
      f.provider = val
      break
    case 'family':
      f.family = val
      break
    case 'tool_call':
      f.toolCall = parseBool(val)
      break
    case 'reasoning':
      f.reasoning = parseBool(val)
      break
    case 'open_weights':
      f.openWeights = parseBool(val)
      break
    case 'structured_output':
      f.structuredOutput = parseBool(val)
      break
    case 'temperature':
      f.temperature = parseBool(val)
      break
  }
}
