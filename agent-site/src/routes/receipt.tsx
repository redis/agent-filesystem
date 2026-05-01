// /receipts/:hash — verifiable receipt for any state-changing action.

import { Link, useParams } from 'react-router-dom'
import { Frame } from '../frame'
import { loadReceipt, loadReceipts } from '../loaders'

export default function ReceiptPage() {
  const { hash = '' } = useParams()
  const r = loadReceipt(hash)
  const all = loadReceipts()

  if (!r) {
    return (
      <Frame
        path="/receipts/:hash"
        realPath={`/receipts/${hash}`}
        meta="not found"
        request={{ method: 'GET', path: `/receipts/${hash}` }}
        json={{ error: 'not_found', resource: 'receipt', hash }}
      >
        <h1>not found</h1>
        <p className="err">no receipt with hash <code>{hash}</code>.</p>
        <h2>recent receipts</h2>
        <ul className="flat">
          {all.map((rx) => (
            <li key={rx.hash}>
              <Link to={`/receipts/${rx.hash}`}>{rx.hash.slice(0, 24)}…</Link>{' '}
              <span className="verb">{rx.verb}</span>{' '}
              <span className="dim">{rx.ts}</span>
            </li>
          ))}
        </ul>
      </Frame>
    )
  }

  return (
    <Frame
      path="/receipts/:hash"
      realPath={`/receipts/${r.hash.slice(0, 24)}…`}
      meta={`${r.verb} · ${r.agent_id} · ${r.ts}`}
      request={{ method: 'GET', path: `/receipts/${r.hash}`, headers: { etag: `w/"${r.action_id}"`, cost_ms: 1 } }}
      json={r}
      cliCommand={['afs', 'receipt', 'show', r.hash, '--json']}
      pyCall={`afs.receipts.get('${r.hash}')`}
      tsCall={`await afs.receipts.get('${r.hash}')`}
    >
      <h1>receipt</h1>

      <dl className="kv">
        <dt>hash</dt>           <dd className="strong">{r.hash}</dd>
        <dt>action_id</dt>      <dd>{r.action_id}</dd>
        <dt>ts</dt>             <dd>{r.ts}</dd>
        <dt>verb</dt>           <dd className="verb">{r.verb}</dd>
        <dt>resource</dt>       <dd className="strong">{r.resource}</dd>
        <dt>agent</dt>          <dd>{r.agent_id}</dd>
        <dt>session</dt>        <dd>{r.session_id}</dd>
        <dt>user</dt>           <dd>{r.user}</dd>
        <dt>before</dt>         <dd>{r.before_ref ?? <span className="dim">(none)</span>}</dd>
        <dt>after</dt>          <dd className="strong">{r.after_ref}</dd>
        <dt>bytes Δ</dt>        <dd className={r.bytes_delta >= 0 ? 'ok' : 'err'}>{r.bytes_delta >= 0 ? '+' : ''}{r.bytes_delta}</dd>
        <dt>cost</dt>           <dd>{r.cost.tokens}tok · {r.cost.bytes}b · {r.cost.ms}ms</dd>
        <dt>signature</dt>      <dd className="dim">{r.signature}</dd>
      </dl>

      <h2>undo</h2>
      <p>
        <code>POST /actions/undo</code> with <code>{`{"undo_token": "${r.undo_token}"}`}</code> reverts this action.
        idempotent. produces a counter-receipt with <code>before/after</code> swapped.
      </p>

      <h2>replay</h2>
      <p className="dim">deterministic re-execution. same args → same hash (modulo timestamp).</p>

      <details open>
        <summary>curl</summary>
        <pre>{r.replay.curl}</pre>
      </details>
      <details>
        <summary>cli</summary>
        <pre>{r.replay.cli}</pre>
      </details>
      <details>
        <summary>mcp</summary>
        <pre>{r.replay.mcp}</pre>
      </details>

      <h2>verification</h2>
      <p>
        signature is <code>ed25519</code> over <code>{`(action_id || ts || verb || resource || after_ref)`}</code>.
        public key at <code>/.well-known/afs-pub-key</code>. clients verify before trusting.
      </p>
    </Frame>
  )
}
