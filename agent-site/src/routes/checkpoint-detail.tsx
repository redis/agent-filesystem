// /checkpoints/:id — single checkpoint, parent chain, diffable, restorable.

import { Link, useParams } from 'react-router-dom'
import { Frame } from '../frame'
import { ActionsInline } from '../actions-row'
import { formatBytes, formatBytesDelta } from '../format'
import { loadCheckpoint, loadCheckpoints } from '../loaders'

export default function CheckpointDetail() {
  const { id = '' } = useParams()
  const cp = loadCheckpoint(id)

  if (!cp) {
    return (
      <Frame
        path="/checkpoints/:id"
        realPath={`/checkpoints/${id}`}
        meta="not found"
        request={{ method: 'GET', path: `/checkpoints/${id}` }}
        json={{ error: 'not_found', resource: 'checkpoint', id }}
      >
        <h1>not found</h1>
        <p className="err">no checkpoint <code>{id}</code>.</p>
      </Frame>
    )
  }

  const chain = parentChain(cp.id)

  return (
    <Frame
      path="/checkpoints/:id"
      realPath={`/checkpoints/${cp.id}`}
      meta={`${cp.workspace_id} · ${cp.file_count} files · ${formatBytes(cp.total_bytes)} · author=${cp.author}`}
      request={{ method: 'GET', path: `/checkpoints/${cp.id}`, headers: { etag: `w/"${cp.id}"`, cost_ms: 3 } }}
      json={{ checkpoint: cp, parent_chain: chain }}
      toolCall={{ name: 'checkpoint_get', args: { workspace_id: cp.workspace_id, checkpoint_id: cp.id } }}
      cliCommand={['afs', 'cp', 'show', cp.workspace_id, cp.name, '--json']}
      pyCall={`afs.workspaces.get('${cp.workspace_id}').checkpoints.get('${cp.id}')`}
      tsCall={`await afs.workspaces.get('${cp.workspace_id}').checkpoints.get('${cp.id}')`}
    >
      <p className="breadcrumb">
        <Link to="/workspaces">/workspaces</Link>
        <span className="sep">/</span>
        <Link to={`/workspaces/${cp.workspace_id}`}>{cp.workspace_id}</Link>
        <span className="sep">/</span>
        <span className="strong">{cp.name}</span>
      </p>
      <h1>{cp.id} · {cp.name}</h1>
      <dl className="kv">
        <dt>workspace</dt>      <dd><Link to={`/workspaces/${cp.workspace_id}`}>{cp.workspace_id}</Link></dd>
        <dt>parent</dt>         <dd>{cp.parent_id ? <Link to={`/checkpoints/${cp.parent_id}`}>{cp.parent_id}</Link> : <span className="dim">(genesis)</span>}</dd>
        <dt>created</dt>        <dd>{cp.created_at}</dd>
        <dt>author</dt>         <dd>{cp.author}</dd>
        <dt>source</dt>         <dd>{cp.source}</dd>
        <dt>manifest hash</dt>  <dd className="strong">{cp.manifest_hash}</dd>
        <dt>contents</dt>       <dd>{cp.file_count} files · {cp.folder_count} folders · {formatBytes(cp.total_bytes)}</dd>
        <dt>delta vs parent</dt><dd>
          <span className={cp.delta_files >= 0 ? 'ok' : 'err'}>{cp.delta_files >= 0 ? '+' : ''}{cp.delta_files} files</span>{', '}
          <span className={cp.delta_bytes >= 0 ? 'ok' : 'err'}>{formatBytesDelta(cp.delta_bytes)}</span>
        </dd>
        {cp.note && <><dt>note</dt><dd className="strong">{cp.note}</dd></>}
      </dl>

      <h2>parent chain</h2>
      {chain.length === 0 ? (
        <p className="dim">(genesis)</p>
      ) : (
        <ol className="flat" style={{ listStyle: 'none' }}>
          {chain.map((p, i) => (
            <li key={p.id}>
              <span className="dim">{'  '.repeat(i)}↳ </span>
              <Link to={`/checkpoints/${p.id}`}>{p.id}</Link>
              {' '}<span className="strong">{p.name}</span>
              <span className="dim"> · {p.created_at}</span>
            </li>
          ))}
        </ol>
      )}

      <div className="actions-row">
        <span className="label">actions:</span>
        <ActionsInline actions={cp._actions} featuredCount={cp._actions.length} />
      </div>

      <h2>diff against</h2>
      <ul className="flat">
        <li>
          working copy:{' '}
          <Link to={`/workspaces/${cp.workspace_id}/diff?base=checkpoint:${cp.id}&head=working-copy`}>
            <code>?base=checkpoint:{cp.id}&amp;head=working-copy</code>
          </Link>
        </li>
        {cp.parent_id && (
          <li>
            parent:{' '}
            <Link to={`/workspaces/${cp.workspace_id}/diff?base=checkpoint:${cp.parent_id}&head=checkpoint:${cp.id}`}>
              <code>?base=checkpoint:{cp.parent_id}&amp;head=checkpoint:{cp.id}</code>
            </Link>
          </li>
        )}
      </ul>
    </Frame>
  )
}

function parentChain(id: string): { id: string; name: string; created_at: string }[] {
  const all = loadCheckpoints()
  const out: { id: string; name: string; created_at: string }[] = []
  let cur = all.find((c) => c.id === id) ?? null
  while (cur && cur.parent_id) {
    const parent = all.find((c) => c.id === cur!.parent_id)
    if (!parent) break
    out.push({ id: parent.id, name: parent.name, created_at: parent.created_at })
    cur = parent
  }
  return out
}
