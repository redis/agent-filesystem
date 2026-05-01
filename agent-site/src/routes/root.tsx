// /  — the manifest. cat /etc/afs.

import { Link } from 'react-router-dom'
import { Frame } from '../frame'
import { currentCapability, loadWorkspaces, loadSessions } from '../loaders'
import { manifest } from '../manifest'

export default function Root() {
  const cap = currentCapability()
  const ws = loadWorkspaces()
  const sessions = loadSessions()

  const json = {
    name: manifest.name,
    version: manifest.version,
    auth: { token: cap.token, scope: cap.scope, profile: cap.profile, expires_at: cap.expires_at },
    workspaces: { count: ws.count, dirty: ws.dirty_count },
    sessions: { count: sessions.length, active: sessions.filter((s) => s.state === 'active').length },
    endpoints: manifest.endpoints,
    headers: manifest.headers,
    media_types: manifest.media_types,
    format_query_param: manifest.format_query_param,
    discovery: '/.well-known/afs-agent-manifest.json',
  }

  return (
    <Frame
      path="/"
      realPath="/"
      meta="manifest. lists every callable. token-bound."
      request={{ method: 'GET', path: '/', headers: { etag: 'w/"manifest-0.1.0"', cost_ms: 1 } }}
      json={json}
      cliCommand={['afs', 'status', '--json']}
      pyCall={'print(afs.status())'}
      tsCall={'console.log(await afs.status())'}
    >
      <h1>agent-filesystem</h1>
      <p className="dim">
        <code>GET /</code> returns the bearer's capability, current workspace and session counts, and an index of every
        callable URL on this surface. Cacheable 60s. JSON form via <code>?format=json</code> or{' '}
        <code>Accept: application/json</code>.
      </p>

      <h2>auth</h2>
      <dl className="kv">
        <dt>token</dt>            <dd className="strong">{cap.token}</dd>
        <dt>scope</dt>            <dd>{cap.scope}{cap.workspace_id ? ` · ${cap.workspace_id}` : ''}</dd>
        <dt>profile</dt>          <dd>{cap.profile} {cap.readonly ? <span className="chip warn">readonly</span> : <span className="chip ok">read+write</span>}</dd>
        <dt>expires</dt>          <dd>{cap.expires_at}</dd>
        <dt>granted tools</dt>    <dd>{cap.granted.join(', ')}</dd>
      </dl>
      <p className="dim">
        Browser session present? The surface auto-handshakes via the Clerk cookie on first load.
        Bring-your-own-token (CLI, scripts, embedded clients): <Link to="/handshake">POST /handshake</Link>.
      </p>

      <h2>state</h2>
      <dl className="kv">
        <dt>workspaces</dt>       <dd>{ws.count} (<span className="err">{ws.dirty_count} dirty</span>) → <Link to="/workspaces">/workspaces</Link></dd>
        <dt>sessions</dt>         <dd>{sessions.length} ({sessions.filter((s) => s.state === 'active').length} active) → <Link to="/sessions">/sessions</Link></dd>
        <dt>activity</dt>         <dd>live stream → <Link to="/activity">/activity</Link></dd>
        <dt>tools</dt>            <dd>14 callables → <Link to="/tools">/tools</Link></dd>
      </dl>

      <h2>endpoints</h2>
      <table>
        <thead><tr><th>method</th><th>path</th><th>role</th></tr></thead>
        <tbody>
          {Object.entries(manifest.endpoints).map(([name, ep]) => (
            <tr key={name}>
              <td className="verb">{(ep as { method: string }).method}</td>
              <td>
                <Link to={resolveLinkable((ep as { path: string }).path)}>{(ep as { path: string }).path}</Link>
              </td>
              <td className="dim">{name.replace(/_/g, ' ')}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2>contracts</h2>
      <dl className="kv">
        <dt>auth</dt>
        <dd><code>Authorization: Bearer &lt;token&gt;</code> · 401 returns a <code>Link: &lt;/why/...&gt;</code> header pointing to a structured rejection</dd>
        <dt>writes</dt>
        <dd><code>Idempotency-Key: &lt;uuid&gt;</code> required on every mutation · response body is a signed receipt at <code>/receipts/&lt;hash&gt;</code></dd>
        <dt>concurrency</dt>
        <dd><code>ETag</code> on every resource · <code>If-Match</code> required on mutations · 412 on conflict</dd>
        <dt>streaming</dt>
        <dd><code>Accept: text/event-stream</code> turns <code>/activity</code> and <code>/sessions</code> into live tails</dd>
        <dt>time-travel</dt>
        <dd><code>?at=&lt;iso-ts&gt;</code> on any list endpoint · <code>?since=&lt;iso-ts&gt;</code> on streams</dd>
        <dt>cost</dt>
        <dd><code>X-AFS-Cost: tokens=N,bytes=N,ms=N</code> on every response · cumulative at <code>/ledger</code></dd>
        <dt>dry-run</dt>
        <dd><code>X-AFS-DryRun: 1</code> short-circuits any mutation; response is the receipt that <em>would</em> have been produced</dd>
        <dt>format</dt>
        <dd><code>?format=json|jsonl|curl|mcp|cli|py|ts</code> renders the same data as code in any of those forms</dd>
      </dl>

      <h2>discovery</h2>
      <p>
        Machine-readable manifest:{' '}
        <a href="/.well-known/afs-agent-manifest.json"><code>/.well-known/afs-agent-manifest.json</code></a>.
        Single fetch, fully describes endpoints, headers, media types, event streams, and auth profiles.
      </p>
    </Frame>
  )
}

const resolveLinkable = (path: string): string => {
  // turn /workspaces/:id into /workspaces (best link to navigate to)
  return path.replace(/\/:[a-zA-Z_]+/g, '').split('?')[0] || '/'
}
