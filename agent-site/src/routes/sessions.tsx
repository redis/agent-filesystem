// /sessions — live agent sessions, killable.

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Frame } from '../frame'
import { loadSessions } from '../loaders'

export default function Sessions() {
  const initial = loadSessions()
  const [sessions, setSessions] = useState(initial)
  const [now, setNow] = useState(Date.now())

  useEffect(() => {
    const handle = setInterval(() => setNow(Date.now()), 1500)
    return () => clearInterval(handle)
  }, [])

  // bump ops_count on active sessions to feel live
  useEffect(() => {
    const handle = setInterval(() => {
      setSessions((prev) =>
        prev.map((s) =>
          s.state === 'active'
            ? { ...s, ops_count: s.ops_count + Math.floor(Math.random() * 4), last_seen: new Date().toISOString() }
            : s,
        ),
      )
    }, 1500)
    return () => clearInterval(handle)
  }, [])

  const kill = (id: string) => {
    setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, state: 'closing' as const } : s)))
    // pretend to close
    setTimeout(() => {
      setSessions((prev) => prev.filter((s) => s.id !== id))
    }, 1200)
  }

  return (
    <Frame
      path="/sessions"
      realPath="/sessions"
      meta={`${sessions.length} sessions · ${sessions.filter((s) => s.state === 'active').length} active · ${sessions.filter((s) => s.state === 'idle').length} idle`}
      request={{ method: 'GET', path: '/sessions', headers: { etag: `w/"sessions-${sessions.length}"`, cost_ms: 3 } }}
      json={{ items: sessions, now: new Date(now).toISOString() }}
      toolCall={{ name: 'session_list', args: {} }}
      cliCommand={['afs', 'status', '--sessions', '--json']}
      pyCall={'for s in afs.sessions.list(): print(s.id, s.state)'}
      tsCall={'(await afs.sessions.list()).forEach(s => console.log(s.id, s.state))'}
    >
      <h1>sessions · live</h1>
      <p className="dim">
        every connected agent. ops_count and last_seen update in real time. <span className="verb">kill</span> immediately
        revokes the token's session-bound capabilities and closes the connection.
      </p>

      <table>
        <thead>
          <tr>
            <th>id</th>
            <th>agent</th>
            <th>workspace</th>
            <th>label</th>
            <th>state</th>
            <th className="num">ops</th>
            <th>idle</th>
            <th>client</th>
            <th>_actions</th>
          </tr>
        </thead>
        <tbody>
          {sessions.map((s) => {
            const idleSec = Math.floor((now - new Date(s.last_seen).getTime()) / 1000)
            return (
              <tr key={s.id}>
                <td className="strong">{s.id}</td>
                <td>{s.agent_id}</td>
                <td><Link to={`/workspaces/${s.workspace_id}`}>{s.workspace_id}</Link></td>
                <td className="dim">{s.label}</td>
                <td>
                  <span className={`chip ${s.state === 'active' ? 'ok' : s.state === 'idle' ? 'info' : 'warn'}`}>{s.state}</span>
                </td>
                <td className="num">{s.ops_count}</td>
                <td className={`num ${idleSec < 5 ? 'ok' : idleSec < 60 ? 'info' : 'warn'}`}>{idleSec}s</td>
                <td className="dim">{s.client}</td>
                <td className="actions">
                  <Link to={`/activity?session=${s.id}`}>tail</Link>{' '}
                  <button onClick={() => kill(s.id)} disabled={s.state === 'closing'}>kill</button>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>

      {sessions.length === 0 && (
        <p className="dim">(no sessions)</p>
      )}

      <h2>open a session</h2>
      <p>
        <code>POST /v1/client/workspaces/&lt;id&gt;/sessions</code> with{' '}
        <code>{`{"agent_id": "...", "label": "...", "client": "..."}`}</code> issues a session id and an SSE channel
        to push events back. heartbeat every 30s via <code>POST /sessions/:id/heartbeat</code>.
      </p>
    </Frame>
  )
}
