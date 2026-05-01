// /workspaces — list. each row carries _actions. no edit pencils.
// live: GET /v1/workspaces. fall back to mock on error.

import { Link } from 'react-router-dom'
import { Frame } from '../frame'
import { ActionsInline } from '../actions-row'
import { loadWorkspaces } from '../loaders'
import { useFetch } from '../hooks'
import { adaptWorkspaceList } from '../adapters'
import type { Workspace } from '../types'

export default function Workspaces() {
  const live = useFetch<{ items: Workspace[] }>('/v1/workspaces')
  const data = live.data
    ? adaptWorkspaceList(live.data as { items: Parameters<typeof adaptWorkspaceList>[0]['items'] })
    : loadWorkspaces()
  const isLive = !!live.data
  const errMsg = live.error ? errString(live.error) : null

  return (
    <Frame
      path="/workspaces"
      realPath="/workspaces"
      meta={`list workspaces · ${data.count} results · ${data.dirty_count} dirty${isLive ? '' : errMsg ? ' · using mocks (api unreachable)' : ' · loading…'}`}
      request={{ method: 'GET', path: '/v1/workspaces', headers: { etag: data.etag, cost_ms: 4 } }}
      json={data}
      toolCall={{ name: 'workspace_list', args: {} }}
      cliCommand={['afs', 'ws', 'list', '--json']}
      pyCall={'for w in afs.workspaces.list(): print(w.id, w.draft_state)'}
      tsCall={'(await afs.workspaces.list()).forEach(w => console.log(w.id, w.draft_state))'}
    >
      {!isLive && errMsg && (
        <p className="dim">
          <span className="chip warn">api unreachable</span>
          {' '}<code>{errMsg}</code>. showing mock data. start the control plane with <code>make web-dev</code>.
        </p>
      )}
      <table>
        <thead>
          <tr>
            <th style={{ minWidth: '22ch' }}>name</th>
            <th>id</th>
            <th>state</th>
            <th className="num">files</th>
            <th className="num">bytes</th>
            <th className="num">cps</th>
            <th>updated</th>
            <th>_actions</th>
          </tr>
        </thead>
        <tbody>
          {data.items.map((w) => (
            <tr key={w.id}>
              <td>
                <Link to={`/workspaces/${w.id}`} className="strong">{w.name}</Link>
              </td>
              <td className="dim">{w.id}</td>
              <td>
                <span className={`chip ${w.draft_state === 'dirty' ? 'warn' : 'ok'}`}>{w.draft_state}</span>
              </td>
              <td className="num">{w.file_count}</td>
              <td className="num">{w.total_bytes.toLocaleString()}</td>
              <td className="num">{w.checkpoint_count}</td>
              <td className="dim">{relTime(w.updated_at)}</td>
              <td className="actions">
                <ActionsInline actions={w._actions} featuredCount={3} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>filters</h2>
      <p className="dim">
        <code>?at=&lt;iso-ts&gt;</code> snapshots the list at a point in time. <code>?status=dirty</code>,{' '}
        <code>?agent=&lt;id&gt;</code>, <code>?database=&lt;id&gt;</code> stack. Cursor pagination via <code>Link</code>{' '}
        header: <code>rel="next"</code> / <code>rel="prev"</code>.
      </p>

      <h2>actions</h2>
      <p className="dim">
        Each row carries <code>_actions: Action[]</code>. <code>GET</code> verbs are direct navigations; mutating verbs
        require <code>Idempotency-Key</code> and a JSON body matching the action's <code>schema</code>. Resolve the
        request body shape from any action's href via <code>?format=mcp</code>.
      </p>

      <h2>create</h2>
      <p>
        <code>POST /workspaces</code> with body{' '}
        <code>{'{ "name": string, "database_id": string? }'}</code>. Returns <code>201</code> + a{' '}
        <Link to="/receipts/sha256:dead7777">receipt</Link> (signed, with <code>undo_token</code>). 409 if{' '}
        <code>name</code> is taken in the bound database.
      </p>
    </Frame>
  )
}

function errString(e: Error | { status?: number; message: string }): string {
  if ('status' in e && e.status) return `${e.status} ${e.message ?? ''}`.trim()
  return (e as Error).message ?? String(e)
}

function relTime(iso: string): string {
  const t = new Date(iso).getTime()
  const now = Date.now()
  const s = Math.floor((now - t) / 1000)
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}
