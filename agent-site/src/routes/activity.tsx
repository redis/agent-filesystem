// /activity — live SSE-shaped tail.
// JS simulates the stream by appending one event every ~1.5s.

import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Frame } from '../frame'
import { formatBytesDelta } from '../format'
import { loadActivity } from '../loaders'
import type { ActivityEvent } from '../types'

export default function Activity() {
  const [params] = useSearchParams()
  const filter = useMemo(
    () => ({
      workspace: params.get('workspace') || undefined,
      agent: params.get('agent') || undefined,
      session: params.get('session') || undefined,
      since: params.get('since') || undefined,
    }),
    [params],
  )

  const initial = useMemo(() => loadActivity(filter), [filter])
  const [stream, setStream] = useState<ActivityEvent[]>(initial)
  const lastIdsRef = useRef<Set<string>>(new Set(initial.map((e) => e.id)))

  useEffect(() => {
    setStream(initial)
    lastIdsRef.current = new Set(initial.map((e) => e.id))
  }, [initial])

  // simulate live tail. cycle through templates, give each a fresh ts and id.
  useEffect(() => {
    const tick = () => {
      const sample = templates[Math.floor(Math.random() * templates.length)]
      // skip if this event would be filtered out
      if (filter.workspace && sample.workspace_id !== filter.workspace) return
      if (filter.agent && sample.agent_id !== filter.agent) return
      if (filter.session && sample.session_id !== filter.session) return

      const now = new Date()
      const event: ActivityEvent = {
        ...sample,
        id: `${now.getTime()}-${Math.floor(Math.random() * 1000)}`,
        ts: now.toISOString(),
      }
      lastIdsRef.current.add(event.id)
      setStream((prev) => [event, ...prev].slice(0, 60))
    }
    const handle = setInterval(tick, 1500)
    return () => clearInterval(handle)
  }, [filter])

  const json = { items: stream, stream: 'live', filter }

  const filterChips = (
    <>
      {filter.workspace && <span className="chip info">workspace={filter.workspace}</span>}
      {filter.agent && <span className="chip info">agent={filter.agent}</span>}
      {filter.session && <span className="chip info">session={filter.session}</span>}
      {filter.since && <span className="chip info">since={filter.since}</span>}
      {!filter.workspace && !filter.agent && !filter.session && !filter.since && <span className="dim">no filter</span>}
    </>
  )

  return (
    <Frame
      path="/activity"
      realPath={'/activity' + paramString(filter)}
      meta={`live · ${stream.length} buffered events · interval=1500ms`}
      request={{
        method: 'GET',
        path: '/activity',
        query: filter as Record<string, string>,
        headers: { cost_ms: 2 },
      }}
      json={json}
      toolCall={{ name: 'activity_tail', args: filter }}
      cliCommand={['afs', 'log', '--follow', ...(filter.workspace ? ['--workspace', filter.workspace] : []), '--json']}
      pyCall={`for ev in afs.activity.tail(${pyArgs(filter)}):\n    print(ev)`}
      tsCall={`for await (const ev of afs.activity.tail(${tsArgs(filter)})) console.log(ev)`}
    >
      <h1>activity · live tail</h1>
      <p className="dim">
        media: <code>application/jsonl</code> over SSE. one event per line. the page synthesizes events client-side; in
        prod the same URL streams from <code>afs:changelog:&lt;workspace&gt;</code> redis streams via SSE. <Link to="?format=jsonl">?format=jsonl</Link>{' '}
        for raw newline-delimited json.
      </p>
      <p>filters: {filterChips}</p>

      <h2>filter</h2>
      <form className="inline" onSubmit={(e) => e.preventDefault()}>
        <FilterLink href="/activity" label="all" active={!filter.workspace && !filter.agent && !filter.session} />
        <FilterLink href="/activity?workspace=payments-portal" label="workspace=payments-portal" active={filter.workspace === 'payments-portal'} />
        <FilterLink href="/activity?workspace=gpt-eval-harness" label="workspace=gpt-eval-harness" active={filter.workspace === 'gpt-eval-harness'} />
        <FilterLink href="/activity?agent=claude-opus-4-7" label="agent=claude-opus-4-7" active={filter.agent === 'claude-opus-4-7'} />
        <FilterLink href="/activity?agent=codex" label="agent=codex" active={filter.agent === 'codex'} />
      </form>

      <h2>stream</h2>
      <table>
        <thead><tr><th>ts</th><th>workspace</th><th>op</th><th>path</th><th className="num">δb</th><th>agent</th><th>session</th></tr></thead>
        <tbody>
          {stream.map((e, i) => (
            <tr key={e.id} className={i === 0 ? 'row-new' : undefined}>
              <td className="dim">{e.ts.slice(11, 23)}</td>
              <td><Link to={`/workspaces/${e.workspace_id}`}>{e.workspace_id}</Link></td>
              <td className="verb">{e.op}</td>
              <td className="strong">{e.path ?? '—'}</td>
              <td className={`num ${(e.bytes_delta ?? 0) >= 0 ? 'ok' : 'err'}`}>{e.bytes_delta != null ? formatBytesDelta(e.bytes_delta) : '—'}</td>
              <td className="dim">{e.agent_id ?? e.user}</td>
              <td className="dim">{e.session_id ?? '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>replay</h2>
      <p>
        <code>?since=&lt;ts&gt;</code> rewinds. <code>?since=2026-05-01T00:00:00Z&amp;agent=codex</code> is the canonical
        "what did codex do today" call. there is no separate <code>/replay</code> endpoint — same url, more filter.
      </p>
    </Frame>
  )
}

function FilterLink({ href, label, active }: { href: string; label: string; active: boolean }) {
  return (
    <Link to={href} className={active ? 'verb' : ''} aria-current={active ? 'page' : undefined}>
      {active ? `[${label}]` : label}
    </Link>
  )
}

function paramString(f: Record<string, string | undefined>) {
  const entries = Object.entries(f).filter(([_, v]) => v)
  if (entries.length === 0) return ''
  return '?' + entries.map(([k, v]) => `${k}=${encodeURIComponent(v as string)}`).join('&')
}

function pyArgs(f: Record<string, string | undefined>) {
  const entries = Object.entries(f).filter(([_, v]) => v)
  if (entries.length === 0) return ''
  return entries.map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', ')
}

function tsArgs(f: Record<string, string | undefined>) {
  const entries = Object.entries(f).filter(([_, v]) => v)
  if (entries.length === 0) return '{}'
  return '{ ' + entries.map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(', ') + ' }'
}

const templates: Omit<ActivityEvent, 'id' | 'ts'>[] = [
  { workspace_id: 'payments-portal', session_id: 'sess-1a2b3c', agent_id: 'claude-opus-4-7', user: 'rowan', source: 'mcp', op: 'file_write', path: 'src/api/charges.ts', bytes_delta: 88, hash: 'sha256:c1a2...' },
  { workspace_id: 'payments-portal', session_id: 'sess-1a2b3c', agent_id: 'claude-opus-4-7', user: 'rowan', source: 'mcp', op: 'file_write', path: 'src/api/refunds.ts', bytes_delta: -42, hash: 'sha256:7d3f...' },
  { workspace_id: 'gpt-eval-harness', session_id: 'sess-4d5e6f', agent_id: 'codex', user: 'rowan', source: 'mcp', op: 'file_write', path: 'evals/v04/probe_004.json', bytes_delta: 1080, hash: 'sha256:9e4a...' },
  { workspace_id: 'scratch-2026-05', session_id: 'sess-7g8h9i', agent_id: 'claude-haiku-4-5', user: 'rowan', source: 'mcp', op: 'file_create', path: 'notes/draft-05.md', bytes_delta: 1290, hash: 'sha256:3b1c...' },
  { workspace_id: 'payments-portal', session_id: 'sess-1a2b3c', agent_id: 'claude-opus-4-7', user: 'rowan', source: 'mcp', op: 'file_replace', path: 'src/api/webhooks/stripe.ts', bytes_delta: 4, hash: 'sha256:aaee...' },
  { workspace_id: 'gpt-eval-harness', session_id: 'sess-4d5e6f', agent_id: 'codex', user: 'rowan', source: 'mcp', op: 'file_write', path: 'evals/v04/scoring.py', bytes_delta: 220, hash: 'sha256:cc11...' },
]
