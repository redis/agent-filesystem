// /tools — every callable. self-describing. try-it form.

import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Frame } from '../frame'
import { loadTool, loadTools } from '../loaders'

export default function Tools() {
  const [params] = useSearchParams()
  const id = params.get('id') || undefined
  const family = params.get('family') || undefined
  const scope = params.get('scope') || undefined

  const tools = useMemo(() => loadTools({ id, family, scope }), [id, family, scope])
  const single = id ? loadTool(id) : null

  if (single) return <SingleTool toolId={id!} />

  return (
    <Frame
      path="/tools"
      realPath={'/tools' + (family ? `?family=${family}` : '') + (scope ? (family ? '&' : '?') + `scope=${scope}` : '')}
      meta={`${tools.length} tools · families=${[...new Set(tools.map((t) => t.family))].join(',')}`}
      request={{ method: 'GET', path: '/tools', query: { ...(family && { family }), ...(scope && { scope }) }, headers: { cost_ms: 1 } }}
      json={{ items: tools }}
      cliCommand={['afs', 'mcp', 'tools', '--json']}
      pyCall={'for t in afs.tools.list(): print(t.id, t.scope)'}
      tsCall={'(await afs.tools.list()).forEach(t => console.log(t.id, t.scope))'}
    >
      <h1>tools</h1>
      <p className="dim">
        every callable. mcp + http + cli surfaces, all in one index. each tool is self-describing — params, types, return shape,
        example invocation. filter by <code>?family=</code> or <code>?scope=</code> or jump to one with <code>?id=</code>.
      </p>

      <h2>filter</h2>
      <form className="inline" onSubmit={(e) => e.preventDefault()}>
        <Link to="/tools" className={!family && !scope ? 'verb' : ''}>{!family && !scope ? '[all]' : 'all'}</Link>
        <span className="dim">family:</span>
        <Link to="/tools?family=workspace" className={family === 'workspace' ? 'verb' : ''}>workspace</Link>
        <Link to="/tools?family=checkpoint" className={family === 'checkpoint' ? 'verb' : ''}>checkpoint</Link>
        <Link to="/tools?family=file" className={family === 'file' ? 'verb' : ''}>file</Link>
        <Link to="/tools?family=admin" className={family === 'admin' ? 'verb' : ''}>admin</Link>
        <span className="dim">scope:</span>
        <Link to="/tools?scope=read" className={scope === 'read' ? 'verb' : ''}>read</Link>
        <Link to="/tools?scope=write" className={scope === 'write' ? 'verb' : ''}>write</Link>
        <Link to="/tools?scope=admin" className={scope === 'admin' ? 'verb' : ''}>admin</Link>
      </form>

      <h2>index</h2>
      <table>
        <thead><tr><th>id</th><th>family</th><th>scope</th><th>profile</th><th>params</th><th>description</th></tr></thead>
        <tbody>
          {tools.map((t) => (
            <tr key={t.id}>
              <td><Link to={`/tools?id=${t.id}`}>{t.id}</Link></td>
              <td className="dim">{t.family}</td>
              <td>
                <span className={`chip ${t.scope === 'read' ? 'info' : t.scope === 'write' ? 'warn' : 'err'}`}>{t.scope}</span>
              </td>
              <td className="dim">{t.profile}</td>
              <td className="dim">{t.params.length}</td>
              <td>{t.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </Frame>
  )
}

function SingleTool({ toolId }: { toolId: string }) {
  const t = loadTool(toolId)
  const [args, setArgs] = useState<string>(t ? JSON.stringify(t.example?.args ?? {}, null, 2) : '{}')
  const [result, setResult] = useState<unknown | null>(null)

  if (!t) {
    return (
      <Frame
        path="/tools"
        realPath={`/tools?id=${toolId}`}
        meta="not found"
        request={{ method: 'GET', path: '/tools', query: { id: toolId } }}
        json={{ error: 'not_found', resource: 'tool', id: toolId }}
      >
        <h1>not found</h1>
        <p className="err">no tool <code>{toolId}</code>. <Link to="/tools">/tools</Link> for index.</p>
      </Frame>
    )
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    let parsed: Record<string, unknown> = {}
    try {
      parsed = JSON.parse(args)
    } catch {
      setResult({ error: 'invalid_json', detail: 'args must be valid json' })
      return
    }
    // synthesize a receipt-shaped response
    const hash = 'sha256:' + Array.from({ length: 12 }, () => Math.floor(Math.random() * 16).toString(16)).join('') + '...'
    setResult({
      ok: true,
      tool: t.id,
      args: parsed,
      result: t.example?.result ?? { synthesized: true },
      receipt: {
        hash,
        action_id: 'act-' + Math.random().toString(36).slice(2, 10),
        ts: new Date().toISOString(),
        cost: { tokens: Math.floor(Math.random() * 400), bytes: Math.floor(Math.random() * 4096), ms: Math.floor(Math.random() * 30) },
      },
      hint: 'this is a mock execution. in prod the receipt is signed and queryable at /receipts/<hash>.',
    })
  }

  return (
    <Frame
      path={`/tools?id=${t.id}`}
      realPath={`/tools?id=${t.id}`}
      meta={`${t.surface} · ${t.family} · ${t.scope} · profile=${t.profile}`}
      request={{ method: 'POST', path: `/tools/${t.id}/execute`, body: { args: t.example?.args ?? {} }, headers: { idempotency_key: '<uuid>' } }}
      json={t}
      toolCall={{ name: t.id, args: t.example?.args ?? {} }}
      cliCommand={['afs', 'mcp', 'call', t.id, '--args', JSON.stringify(t.example?.args ?? {})]}
      pyCall={`afs.tools.${t.id}(${pyKwargs(t.example?.args)})`}
      tsCall={`await afs.tools.${camel(t.id)}(${tsObj(t.example?.args)})`}
    >
      <h1>{t.id}</h1>
      <p className="dim">{t.description}</p>

      <dl className="kv">
        <dt>surface</dt>   <dd>{t.surface}</dd>
        <dt>family</dt>    <dd>{t.family}</dd>
        <dt>scope</dt>     <dd><span className={`chip ${t.scope === 'read' ? 'info' : t.scope === 'write' ? 'warn' : 'err'}`}>{t.scope}</span></dd>
        <dt>profile</dt>   <dd>{t.profile} <span className="dim">(min capability required)</span></dd>
        <dt>returns</dt>   <dd className="strong">{t.returns}</dd>
      </dl>

      <h2>params</h2>
      {t.params.length === 0 ? (
        <p className="dim">(none — call with empty args)</p>
      ) : (
        <table>
          <thead><tr><th>name</th><th>type</th><th>required</th><th>description</th></tr></thead>
          <tbody>
            {t.params.map((p) => (
              <tr key={p.name}>
                <td className="strong">{p.name}</td>
                <td className="dim">{p.type}</td>
                <td>{p.required ? <span className="err">yes</span> : <span className="dim">no</span>}</td>
                <td>{p.description ?? <span className="dim">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2>try it</h2>
      <p className="dim">submit args; receive a synthetic receipt. real execution is gated on your token's profile (<code>{t.profile}</code> minimum).</p>
      <form onSubmit={submit}>
        <textarea
          value={args}
          onChange={(e) => setArgs(e.target.value)}
          spellCheck={false}
        />
        <div style={{ marginTop: '0.5rem' }}>
          <button type="submit">execute</button>{' '}
          <span className="dim">→ POST /tools/{t.id}/execute</span>
        </div>
      </form>

      {result !== null && (
        <>
          <h2>result</h2>
          <pre>{JSON.stringify(result, null, 2)}</pre>
          {(result as { receipt?: { hash: string } }).receipt?.hash && (
            <p>
              receipt: <Link to={`/receipts/${(result as { receipt: { hash: string } }).receipt.hash}`}>
                <code>{(result as { receipt: { hash: string } }).receipt.hash}</code>
              </Link>
            </p>
          )}
        </>
      )}

      <h2>example</h2>
      {t.example && (
        <>
          <h3>args</h3>
          <pre>{JSON.stringify(t.example.args, null, 2)}</pre>
          <h3>result</h3>
          <pre>{JSON.stringify(t.example.result, null, 2)}</pre>
        </>
      )}

      <p className="dim" style={{ marginTop: '1.5rem' }}>
        ← <Link to="/tools">back to index</Link>
      </p>
    </Frame>
  )
}

function pyKwargs(args: unknown): string {
  if (!args || typeof args !== 'object') return ''
  return Object.entries(args).map(([k, v]) => `${k}=${JSON.stringify(v)}`).join(', ')
}
function tsObj(args: unknown): string {
  if (!args || typeof args !== 'object' || Object.keys(args).length === 0) return '{}'
  return '{ ' + Object.entries(args).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(', ') + ' }'
}
function camel(s: string) {
  return s.replace(/_([a-z])/g, (_, c) => c.toUpperCase())
}
