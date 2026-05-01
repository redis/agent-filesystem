// /why/:action_id — machine-readable reason an action was rejected.

import { Link, useParams } from 'react-router-dom'
import { Frame } from '../frame'
import { loadAllWhy, loadWhy } from '../loaders'

export default function WhyPage() {
  const { actionId = '' } = useParams()
  const w = loadWhy(actionId)
  const all = loadAllWhy()

  if (!w) {
    return (
      <Frame
        path="/why/:action_id"
        realPath={`/why/${actionId}`}
        meta="not found"
        request={{ method: 'GET', path: `/why/${actionId}` }}
        json={{ error: 'not_found', resource: 'rejection', action_id: actionId }}
      >
        <h1>not found</h1>
        <p className="err">no rejection record for action <code>{actionId}</code>. either the action succeeded, or its retention window expired.</p>
        <h2>recent rejections</h2>
        <ul className="flat">
          {all.map((r) => (
            <li key={r.action_id}>
              <Link to={`/why/${r.action_id}`}>{r.action_id}</Link>{' '}
              <span className="dim">{r.ts}</span>{' '}
              <span className="err">{r.reason}</span>
            </li>
          ))}
        </ul>
      </Frame>
    )
  }

  return (
    <Frame
      path="/why/:action_id"
      realPath={`/why/${w.action_id}`}
      meta={`rejected · ${w.ts}`}
      request={{ method: 'GET', path: `/why/${w.action_id}`, headers: { cost_ms: 1 } }}
      json={w}
      cliCommand={['afs', 'why', w.action_id, '--json']}
      pyCall={`afs.why('${w.action_id}')`}
      tsCall={`await afs.why('${w.action_id}')`}
    >
      <h1>rejection</h1>
      <p className="dim">
        agents fail silently when they don't know why. this endpoint exists so they don't have to guess.
      </p>

      <dl className="kv">
        <dt>action_id</dt>           <dd className="strong">{w.action_id}</dd>
        <dt>ts</dt>                  <dd>{w.ts}</dd>
        <dt>rejected</dt>            <dd className="err">true</dd>
        <dt>reason</dt>              <dd className="err">{w.reason}</dd>
        <dt>required capability</dt> <dd className="strong">{w.required_capability}</dd>
        <dt>current capability</dt>  <dd>{w.current_capability}</dd>
        <dt>suggested fix</dt>       <dd>{w.suggested_fix}</dd>
        <dt>retry after</dt>         <dd>{w.retry_after === null ? <span className="dim">(never — fix capability first)</span> : `${w.retry_after}s`}</dd>
        <dt>related</dt>             <dd>{w.related_action ? <Link to={`/why/${w.related_action}`}>{w.related_action}</Link> : <span className="dim">(none)</span>}</dd>
      </dl>

      <h2>shape</h2>
      <p>
        every rejection follows this shape. <code>required_capability</code> is the smallest capability that would have
        succeeded. <code>suggested_fix</code> is a literal command an agent can execute. <code>retry_after</code> is null
        when the cause is structural (wrong scope, wrong profile) and a number of seconds when it's transient (etag stale,
        rate-limited).
      </p>
    </Frame>
  )
}
