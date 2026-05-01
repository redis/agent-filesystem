// /workspaces/:id — flat single page. no tabs. state digest, checkpoints, sessions, activity tail, every action.
// live: GET /v1/workspaces/:id returns the workspace + embedded checkpoints + sessions in one shot.

import { Link, useParams } from 'react-router-dom'
import { Frame } from '../frame'
import { ActionsInline } from '../actions-row'
import { loadWorkspace, loadCheckpoints, loadSessions, loadActivity } from '../loaders'
import { useFetch } from '../hooks'
import { adaptWorkspaceDetail, type RawWorkspaceDetail } from '../adapters'
import { APIError } from '../api'
import type { ActivityEvent, Checkpoint, Session, Workspace } from '../types'

export default function WorkspaceDetail() {
  const { id = '' } = useParams()
  const live = useFetch<RawWorkspaceDetail>(id ? `/v1/workspaces/${id}` : null)

  // 404 path: no live record AND no mock for that id.
  const liveNotFound = live.error instanceof APIError && live.error.status === 404
  const mockFallback = liveNotFound || (!live.data && live.error)

  if (live.loading && !live.data) {
    return (
      <Frame
        path="/workspaces/:id"
        realPath={`/workspaces/${id}`}
        meta="loading…"
        request={{ method: 'GET', path: `/v1/workspaces/${id}` }}
        json={{ status: 'loading' }}
      >
        <p className="dim">fetching <code>/v1/workspaces/{id}</code>…</p>
      </Frame>
    )
  }

  if (liveNotFound) {
    return (
      <Frame
        path="/workspaces/:id"
        realPath={`/workspaces/${id}`}
        meta="not found"
        request={{ method: 'GET', path: `/v1/workspaces/${id}`, headers: { cost_ms: 1 } }}
        json={{ error: 'not_found', resource: 'workspace', id }}
      >
        <h1>not found</h1>
        <p className="err">no workspace with id <code>{id}</code>. <Link to="/workspaces">/workspaces</Link>.</p>
      </Frame>
    )
  }

  let w: Workspace
  let checkpoints: Checkpoint[]
  let sessions: Session[]
  let activity: ActivityEvent[]
  let isLive: boolean
  let errMsg: string | null = null
  if (live.data) {
    const detail = adaptWorkspaceDetail(live.data)
    w = detail.workspace
    checkpoints = detail.checkpoints
    sessions = detail.sessions
    activity = []  // not embedded yet; live activity stays mocked for now
    isLive = true
  } else {
    // api unreachable → fall back to mock for offline browsing
    const mock = loadWorkspace(id)
    if (!mock) {
      return (
        <Frame
          path="/workspaces/:id"
          realPath={`/workspaces/${id}`}
          meta="not found (api unreachable, no mock either)"
          request={{ method: 'GET', path: `/v1/workspaces/${id}` }}
          json={{ error: 'not_found', resource: 'workspace', id }}
        >
          <h1>not found</h1>
          <p className="err">no workspace with id <code>{id}</code>. <Link to="/workspaces">/workspaces</Link>.</p>
        </Frame>
      )
    }
    w = mock
    checkpoints = loadCheckpoints(w.id)
    sessions = loadSessions(w.id)
    activity = loadActivity({ workspace: w.id }).slice(0, 8)
    isLive = false
    errMsg = live.error ? (live.error.message || String(live.error)) : null
  }

  return (
    <Frame
      path="/workspaces/:id"
      realPath={`/workspaces/${w.id}`}
      meta={`${w.draft_state} · ${w.file_count} files · ${w.total_bytes.toLocaleString()}b · ${w.checkpoint_count} checkpoints${isLive ? '' : ' · using mocks (api unreachable)'}`}
      request={{ method: 'GET', path: `/v1/workspaces/${w.id}`, headers: { etag: w.etag, cost_ms: 6 } }}
      json={{ workspace: w, checkpoints, sessions, recent_activity: activity }}
      toolCall={{ name: 'workspace_get', args: { id: w.id } }}
      cliCommand={['afs', 'ws', 'info', w.name, '--json']}
      pyCall={`afs.workspaces.get('${w.name}').summary()`}
      tsCall={`await afs.workspaces.get('${w.name}').summary()`}
    >
      <p className="breadcrumb">
        <Link to="/workspaces">/workspaces</Link>
        <span className="sep">/</span>
        <span className="strong">{w.name}</span>
      </p>
      <h1>{w.name}</h1>
      <dl className="kv">
        <dt>name</dt>           <dd className="strong">{w.name}</dd>
        <dt>id</dt>             <dd className="dim">{w.id}</dd>
        <dt>database</dt>       <dd>{w.database_id}</dd>
        <dt>state</dt>          <dd>
          <span className={`chip ${w.draft_state === 'dirty' ? 'warn' : 'ok'}`}>{w.draft_state}</span>
        </dd>
        <dt>etag</dt>           <dd className="dim">{w.etag} <span className="dim">(use If-Match for safe writes)</span></dd>
        <dt>files</dt>          <dd>{w.file_count} files · {w.folder_count} folders · {w.total_bytes.toLocaleString()}b</dd>
        <dt>last cp</dt>        <dd>{w.last_checkpoint_at ?? <span className="dim">(never)</span>}</dd>
        <dt>updated</dt>        <dd>{w.updated_at}</dd>
      </dl>

      <div className="actions-row">
        <span className="label">actions:</span>
        <ActionsInline actions={w._actions} featuredCount={w._actions.length} />
      </div>

      <h2>checkpoints ({checkpoints.length})</h2>
      {checkpoints.length === 0 ? (
        <p className="dim">no checkpoints. <code>POST /workspaces/{w.id}/checkpoints</code> to save the live state.</p>
      ) : (
        <table>
          <thead><tr><th>id</th><th>name</th><th>parent</th><th className="num">δ files</th><th className="num">δ bytes</th><th>created</th></tr></thead>
          <tbody>
            {checkpoints.map((c) => (
              <tr key={c.id}>
                <td><Link to={`/checkpoints/${c.id}`}>{c.id}</Link></td>
                <td className="strong">{c.name}</td>
                <td className="dim">{c.parent_id ?? '—'}</td>
                <td className={`num ${c.delta_files >= 0 ? 'ok' : 'err'}`}>{c.delta_files >= 0 ? '+' : ''}{c.delta_files}</td>
                <td className={`num ${c.delta_bytes >= 0 ? 'ok' : 'err'}`}>{c.delta_bytes >= 0 ? '+' : ''}{c.delta_bytes}</td>
                <td className="dim">{c.created_at}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>active sessions ({sessions.length})</h2>
      {sessions.length === 0 ? (
        <p className="dim">no sessions attached.</p>
      ) : (
        <table>
          <thead><tr><th>id</th><th>agent</th><th>label</th><th>state</th><th className="num">ops</th></tr></thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={s.id}>
                <td className="strong">{s.id}</td>
                <td>{s.agent_id}</td>
                <td className="dim">{s.label}</td>
                <td><span className={`chip ${s.state === 'active' ? 'ok' : s.state === 'idle' ? 'info' : 'warn'}`}>{s.state}</span></td>
                <td className="num">{s.ops_count}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>recent activity</h2>
      <p className="dim">→ <Link to={`/activity?workspace=${w.id}`}>tail full stream</Link></p>
      {activity.length === 0 ? (
        <p className="dim">(quiet)</p>
      ) : (
        <table>
          <thead><tr><th>ts</th><th>op</th><th>path</th><th className="num">δb</th><th>agent</th></tr></thead>
          <tbody>
            {activity.map((e) => (
              <tr key={e.id}>
                <td className="dim">{e.ts.slice(11, 23)}</td>
                <td className="verb">{e.op}</td>
                <td className="strong">{e.path ?? '—'}</td>
                <td className={`num ${(e.bytes_delta ?? 0) >= 0 ? 'ok' : 'err'}`}>{e.bytes_delta != null ? (e.bytes_delta >= 0 ? '+' : '') + e.bytes_delta : '—'}</td>
                <td className="dim">{e.agent_id ?? e.user}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>diff</h2>
      <p>
        <code>GET /workspaces/{w.id}/diff?base=&lt;ref&gt;&amp;head=&lt;ref&gt;</code>{' '}
        — refs are <code>checkpoint:&lt;id&gt;</code>, <code>working-copy</code>, or <code>head</code>.
      </p>

      <h2>files</h2>
      <p>
        <code>GET /workspaces/{w.id}/files?glob=**/*.ts&amp;grep=stripe</code> · returns matched paths and content hashes.
      </p>
    </Frame>
  )
}
