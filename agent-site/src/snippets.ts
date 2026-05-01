// snippets — renderers that emit code in a given format.
// every page calls these to produce equivalent client snippets for the same call.
// one source of truth: a RequestShape.

import type { RequestShape } from './types'

const BASE = 'https://afs.cloud'

const queryString = (q?: Record<string, string>) => {
  if (!q) return ''
  const entries = Object.entries(q).filter(([_, v]) => v !== undefined && v !== '')
  if (entries.length === 0) return ''
  return '?' + entries.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&')
}

export const curl = (req: RequestShape): string => {
  const url = `${BASE}${req.path}${queryString(req.query)}`
  const lines: string[] = [`curl -sS '${url}' \\`]
  if (req.method !== 'GET') lines.push(`  -X ${req.method} \\`)
  lines.push(`  -H 'authorization: bearer $AFS_TOKEN' \\`)
  lines.push(`  -H 'accept: application/json' \\`)
  if (req.body) {
    lines.push(`  -H 'content-type: application/json' \\`)
    lines.push(`  -H 'idempotency-key: $(uuidgen)' \\`)
    lines.push(`  -d '${JSON.stringify(req.body)}'`)
  } else {
    // remove the trailing backslash of last line
    lines[lines.length - 1] = lines[lines.length - 1].replace(/ \\$/, '')
  }
  return lines.join('\n')
}

// MCP tool-call shape. Only meaningful for endpoints that map to a tool.
export const mcp = (toolName: string, args: Record<string, unknown> = {}): string => {
  const payload = {
    jsonrpc: '2.0',
    id: 1,
    method: 'tools/call',
    params: { name: toolName, arguments: args },
  }
  return JSON.stringify(payload, null, 2)
}

export const cli = (parts: string[]): string => {
  return parts.map((p) => (p.includes(' ') ? `'${p}'` : p)).join(' ')
}

export const py = (call: string): string => {
  return [
    `from afs import AFS`,
    `afs = AFS()  # reads $AFS_TOKEN`,
    call,
  ].join('\n')
}

export const ts = (call: string): string => {
  return [
    `import { AFS } from 'afs'`,
    `const afs = new AFS()  // reads $AFS_TOKEN`,
    call,
  ].join('\n')
}

// json renders the canonical body the endpoint would return.
export const json = (data: unknown): string => JSON.stringify(data, null, 2)

// jsonl renders an array as newline-delimited JSON, the canonical agent-friendly form.
export const jsonl = (items: unknown[]): string => items.map((i) => JSON.stringify(i)).join('\n')
