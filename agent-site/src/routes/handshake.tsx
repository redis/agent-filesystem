// /handshake — replaces login + signup + /capabilities.
// POST a token, get a capability descriptor. one call. no email. no captcha.

import { useState } from 'react'
import { Frame } from '../frame'
import { currentCapability } from '../loaders'

export default function Handshake() {
  const [token, setToken] = useState('')
  const [profile, setProfile] = useState('workspace-rw')
  const [result, setResult] = useState<unknown | null>(null)
  const cap = currentCapability()

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    // synthetic: any token bound to the requested profile.
    setResult({
      token: token || cap.token,
      scope: 'workspace',
      workspace_id: 'payments-portal',
      profile,
      readonly: profile.endsWith('-ro'),
      expires_at: '2026-05-08T00:00:00Z',
      granted: cap.granted,
      receipt_hash: 'sha256:hs-' + Math.random().toString(36).slice(2, 10),
    })
  }

  const json = result ?? {
    method: 'POST /handshake',
    body: { token: '<your token>', profile },
    response_shape: 'Capability',
    note: 'no token? POST { profile, ttl_seconds } from any pre-authorized session and the surface mints one.',
  }

  return (
    <Frame
      path="/handshake"
      realPath="/handshake"
      meta="token in. capability descriptor out."
      request={{ method: 'POST', path: '/handshake', body: { token: '$AFS_TOKEN', profile }, headers: { idempotency_key: 'hs-${uuid}' } }}
      json={json}
      toolCall={{ name: 'afs_status', args: {} }}
      cliCommand={['afs', 'auth', 'login', '--token', '$AFS_TOKEN']}
      pyCall={`cap = afs.handshake(profile="${profile}")\nprint(cap.granted)`}
      tsCall={`const cap = await afs.handshake({ profile: '${profile}' })\nconsole.log(cap.granted)`}
    >
      <h1>handshake</h1>
      <p className="dim">
        <code>POST /handshake</code> with a bearer token and a profile name. Response is a Capability descriptor:
        <code> {`{ token, scope, workspace_id, profile, readonly, expires_at, granted: string[] }`}</code>. Attach the
        token to subsequent calls via <code>Authorization: Bearer</code>. Idempotent on the same input.
      </p>

      <h2>browser auth bridge</h2>
      <p className="dim">
        On <code>afs.cloud/agent</code>, the surface auto-handshakes on first load: it forwards the Clerk session cookie
        to <code>POST /v1/auth/exchange</code> (<code>credentials: 'include'</code>), receives a session-scoped token,
        and writes it to <code>localStorage</code>. No login form here. If the cookie is missing or expired you're bounced
        to <code>afs.cloud/login?return_to=/agent/...</code> (the existing Clerk page) and back. This page's form below
        is for the bring-your-own-token cases: CLI, scripts, embedded clients.
      </p>

      <h2>request</h2>
      <form className="inline" onSubmit={submit}>
        <label>
          <span className="dim">token: </span>
          <input
            type="text"
            value={token}
            placeholder="$AFS_TOKEN (leave blank to use demo cap)"
            onChange={(e) => setToken(e.target.value)}
            size={48}
          />
        </label>
        <label>
          <span className="dim">profile: </span>
          <select value={profile} onChange={(e) => setProfile(e.target.value)}>
            <option value="workspace-ro">workspace-ro</option>
            <option value="workspace-rw">workspace-rw</option>
            <option value="workspace-rw-checkpoint">workspace-rw-checkpoint</option>
            <option value="admin-ro">admin-ro</option>
            <option value="admin-rw">admin-rw</option>
          </select>
        </label>
        <button type="submit">handshake</button>
      </form>

      <h2>response</h2>
      {result ? (
        <pre>{JSON.stringify(result, null, 2)}</pre>
      ) : (
        <p className="dim">no response yet. submit the form above.</p>
      )}

      <h2>profiles</h2>
      <table>
        <thead><tr><th>profile</th><th>scope</th><th>covers</th></tr></thead>
        <tbody>
          <tr><td className="verb">workspace-ro</td><td>read</td><td className="dim">file_read, file_list, file_glob, file_grep, workspace_list</td></tr>
          <tr><td className="verb">workspace-rw</td><td>read+write</td><td className="dim">+ file_write, file_replace, file_patch, file_create_exclusive, file_insert, file_delete_lines</td></tr>
          <tr><td className="verb">workspace-rw-checkpoint</td><td>read+write+cp</td><td className="dim">+ checkpoint_create, checkpoint_restore, workspace_fork</td></tr>
          <tr><td className="verb">admin-ro</td><td>admin read</td><td className="dim">+ workspace_list (all), agent_list, mcp_token_list</td></tr>
          <tr><td className="verb">admin-rw</td><td>everything</td><td className="dim">+ workspace_delete, mcp_token_issue, mcp_token_revoke</td></tr>
        </tbody>
      </table>

      <h2>errors</h2>
      <p className="dim">
        Bad or expired token → <code>401</code> with <code>Link: &lt;/why/&lt;action_id&gt;&gt;; rel="why"</code>.
        Profile not granted to this token → <code>403</code>, same Link header. Example:{' '}
        <a href="/why/act-2026-05-01-105930-99fe">/why/act-2026-05-01-105930-99fe</a>.
      </p>
    </Frame>
  )
}
