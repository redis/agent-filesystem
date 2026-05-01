// frame.tsx — page chrome shared by every route.
// nav, head bar, footer with format toggle.
//
// the format toggle is a stack of <details> elements — works without JS.

import { ReactNode, useEffect, useState } from 'react'
import { Link, useLocation, useSearchParams } from 'react-router-dom'
import type { RequestShape } from './types'
import * as snip from './snippets'

const NAV: { href: string; label: string }[] = [
  { href: '/', label: '/(root)' },
  { href: '/handshake', label: '/handshake' },
  { href: '/workspaces', label: '/workspaces' },
  { href: '/activity', label: '/activity' },
  { href: '/sessions', label: '/sessions' },
  { href: '/tools', label: '/tools' },
]

export function TopNav() {
  const loc = useLocation()
  return (
    <nav className="topnav" aria-label="primary">
      <Link to="/" className="brandmark" aria-label="agent-filesystem · home">
        <RedisMark />
      </Link>
      <span className="dim">·</span>
      <span className="dim">agent-filesystem/0.1.0</span>
      <span style={{ flex: 1 }} />
      {NAV.map((n) => {
        const active = n.href === '/' ? loc.pathname === '/' : loc.pathname.startsWith(n.href)
        return (
          <Link key={n.href} to={n.href} aria-current={active ? 'page' : undefined}>
            {n.label}
          </Link>
        )
      })}
    </nav>
  )
}

function RedisMark() {
  // Full Redis wordmark, sourced from @redis-ui/icons (RedisLogoLightFull, dark-mode variant).
  // Aspect ratio 371:115 ≈ 3.23:1.
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 371 115"
      width="76"
      height="22"
      role="img"
      aria-label="redis"
      style={{ display: 'block' }}
    >
      <path
        fill="#FF4438"
        d="M303.443 27.087c3.508-6.178 9.248-12.83 11.321-14.89 9.568 3.96 18.498 12.038 17.222 14.098-3.668 6.019-9.249 12.83-11.322 14.89-9.567-3.96-18.497-11.88-17.221-14.098Zm67.451 27.245c-1.116 5.386-7.813 11.405-10.684 12.355-2.392-5.069-5.102-8.078-7.654-8.078-3.189 0-3.348 2.217-3.348 5.069 0 5.069 3.667 16.157 3.667 27.72 0 12.672-8.93 22.018-22.643 22.018-12.558 0-19.497-8.177-22.592-21.24-8.203 14.601-20.198 21.24-29.392 21.24-14.373 0-17.756-10.553-17.415-21.257-5.775 10.139-16.892 21.257-27.553 21.257-10.883 0-14.727-9.41-13.846-20.369-6.52 12.066-18.316 20.369-29.686 20.369-12.34 0-18.447-9.737-16.471-21.806-8.301 10.135-23.753 21.806-39.819 21.806-18.318 0-26.291-9.812-27.236-22.108-8.842 14.076-20.758 22.583-34.953 22.583-20.49 0-27.82-18.1-28.887-32.906-7.594 10.102-16.132 20.583-26.605 32.273-1.116 1.108-2.073 1.742-3.19 1.742C8.92 115 1.426 98.843.948 92.824c4.253-6.557 38.994-43.8 53.474-59.84-9.782 2.932-19.867 8.78-32.585 17.863-2.232 1.584-8.451-12.83-8.292-23.919C28.214 16.157 50.54 9.346 68.558 9.346c25.195 0 39.706 13.94 39.706 33.264 0 16.157-13.554 33.898-33.328 34.532-10.28.265-16.87-5.468-20.24-12.547.402 10.949 6.133 24.427 21.516 24.427 17.86 0 25.833-11.405 39.227-28.037 10.206-12.514 22.006-23.602 39.227-23.602 10.525 0 17.7 6.494 17.7 16.315 0 11.88-14.032 28.354-33.646 28.354-3.35 0-6.403-.438-8.98-1.305-.065.5-.109.991-.109 1.464 0 5.544 2.073 8.87 11.162 8.87 13.395 0 25.992-7.92 41.301-26.453 14.989-18.216 26.31-26.136 38.27-26.136 8.073 0 14.2 4.346 16.901 11.666C253.293 27.162 266.89 10.868 278.407 0c11.322 4.752 19.454 14.098 17.222 15.999-8.451 7.603-36.676 38.174-47.838 56.39-2.87 4.753-5.581 9.98-5.581 12.515 0 2.376 1.435 3.168 3.03 3.168 10.524 0 35.559-33.899 49.751-48.788 8.93 3.643 18.019 11.405 15.787 14.098-11.8 13.939-20.73 25.344-20.73 31.838 0 1.743.638 2.852 3.03 2.852 4.465 0 8.61-3.96 15.467-12.356 1.435-1.742 3.189-1.742 4.306.95 3.029 7.287 7.494 11.247 11.002 11.247 4.146 0 6.219-3.643 6.219-9.187 0-6.653-1.435-15.207-1.435-19.008 0-12.831 9.568-20.276 21.527-20.276 8.93 0 16.903 4.277 20.73 14.89ZM76.114 31.226c-5.37 8.338-10.368 16.127-15.418 23.723 2.744 1.531 6.215 2.71 10.732 2.71 8.452 0 17.7-4.594 17.7-13.94 0-5.672-3.543-10.9-13.013-12.493Zm56.051 42.538c1.676.643 3.644 1.002 5.758 1.002 11.322 0 18.976-8.554 18.976-14.256 0-2.535-1.595-4.277-4.146-4.277-6.398 0-16.047 8.918-20.588 17.53Zm94.896-11.829c0-3.168-1.754-5.069-4.624-5.069-9.408 0-23.6 17.741-23.6 26.612 0 2.851 1.594 4.752 4.943 4.752 10.365 0 23.281-18.691 23.281-26.295Z"
      />
    </svg>
  )
}

type FrameProps = {
  path: string                      // /workspaces/:id (canonical, with placeholders)
  realPath: string                  // /workspaces/payments-portal (actual)
  meta?: string                     // one-line summary under path
  request: RequestShape
  json: unknown                     // canonical JSON for this view
  toolCall?: { name: string; args: Record<string, unknown> } // if this maps to an MCP tool
  cliCommand?: string[]             // afs ... args
  pyCall?: string
  tsCall?: string
  children: ReactNode
}

export function Frame(props: FrameProps) {
  const { path, realPath, meta, request, json, toolCall, cliCommand, pyCall, tsCall, children } = props
  const [params, setParams] = useSearchParams()
  const requestedFormat = params.get('format')

  // Sync ?format= to the visible format. JS-only enhancement.
  const [openFormat, setOpenFormat] = useState<string | null>(requestedFormat)
  useEffect(() => {
    setOpenFormat(requestedFormat)
  }, [requestedFormat])

  const setFormat = (f: string | null) => {
    const next = new URLSearchParams(params)
    if (f) next.set('format', f)
    else next.delete('format')
    setParams(next, { replace: true })
  }

  // If ?format=json, the entire page becomes JSON — agents land here directly.
  if (requestedFormat === 'json') {
    return <pre style={{ marginTop: 0 }}>{snip.json(json)}</pre>
  }
  if (requestedFormat === 'jsonl' && Array.isArray((json as { items?: unknown[] }).items)) {
    return <pre style={{ marginTop: 0 }}>{snip.jsonl((json as { items: unknown[] }).items)}</pre>
  }

  return (
    <div>
      <TopNav />

      <header>
        <CommandLine cliCommand={cliCommand} request={request} path={path} />
        {meta && (
          <div className="dim">
            <span>│ </span>
            <span>{meta}</span>
          </div>
        )}
        <div className="dim">
          <span>│ </span>
          <span className="k">over: </span>
          <span className="verb">{request.method}</span>{' '}
          <span className="strong">{path}</span>
        </div>
        <div className="dim">├{'─'.repeat(60)}</div>
      </header>

      <main>{children}</main>

      <footer className="frame-footer">
        <div className="dim">├{'─'.repeat(60)}</div>
        <RequestRow request={request} realPath={realPath} />
        <FormatToggle
          openFormat={openFormat}
          setOpenFormat={(f) => {
            setOpenFormat(f)
            setFormat(f)
          }}
          request={{ ...request, path: realPath }}
          json={json}
          toolCall={toolCall}
          cliCommand={cliCommand}
          pyCall={pyCall}
          tsCall={tsCall}
        />
        <div className="dim">└{'─'.repeat(60)}</div>
      </footer>
    </div>
  )
}

function CommandLine({ cliCommand, request, path }: { cliCommand?: string[]; request: RequestShape; path: string }) {
  const cliText = cliCommand && cliCommand.length > 0 ? `$ ${cliCommand.join(' ')}` : null
  const fallback = `${request.method} ${path}`
  const head = cliText ?? fallback
  const dashes = '─'.repeat(Math.max(2, 70 - head.length - 4))
  return (
    <div className="dim">
      <span>┌─ </span>
      <span style={{ color: 'var(--accent)', fontWeight: 600 }}>{head}</span>
      <span> {dashes}</span>
    </div>
  )
}

function RequestRow({ request, realPath }: { request: RequestShape; realPath: string }) {
  const h = request.headers ?? {}
  const cost = [
    h.cost_ms !== undefined ? `${h.cost_ms}ms` : null,
    h.cost_bytes !== undefined ? `${h.cost_bytes}b` : null,
    h.cost_tokens !== undefined ? `${h.cost_tokens}tok` : null,
  ].filter(Boolean).join(' ')
  return (
    <div className="dim" style={{ display: 'flex', gap: '2ch', flexWrap: 'wrap' }}>
      <span>│</span>
      <span>
        <span className="k">transport:</span>{' '}
        <span className="verb">{request.method}</span>{' '}
        <span className="v">{realPath}</span>
      </span>
      {h.etag && <span><span className="k">etag:</span> <span className="v">{h.etag}</span></span>}
      {cost && <span><span className="k">cost:</span> <span className="v">{cost}</span></span>}
      {h.idempotency_key && <span><span className="k">idem:</span> <span className="v">{h.idempotency_key}</span></span>}
    </div>
  )
}

type ToggleProps = {
  openFormat: string | null
  setOpenFormat: (f: string | null) => void
  request: RequestShape
  json: unknown
  toolCall?: { name: string; args: Record<string, unknown> }
  cliCommand?: string[]
  pyCall?: string
  tsCall?: string
}

function FormatToggle({ openFormat, setOpenFormat, request, json, toolCall, cliCommand, pyCall, tsCall }: ToggleProps) {
  // CLI is the canonical surface; REST is the transport. Order reflects that.
  const items: { key: string; label: string; render: () => string; available: boolean }[] = [
    { key: 'cli',  label: 'cli',  available: !!cliCommand, render: () => cliCommand ? snip.cli(cliCommand) : '(no CLI form)' },
    { key: 'mcp',  label: 'mcp',  available: !!toolCall, render: () => toolCall ? snip.mcp(toolCall.name, toolCall.args) : '(no MCP tool maps to this URL)' },
    { key: 'json', label: 'json', available: true, render: () => snip.json(json) },
    { key: 'curl', label: 'curl', available: true, render: () => snip.curl(request) },
    { key: 'py',   label: 'py',   available: !!pyCall, render: () => pyCall ? snip.py(pyCall) : '(no python sdk form)' },
    { key: 'ts',   label: 'ts',   available: !!tsCall, render: () => tsCall ? snip.ts(tsCall) : '(no typescript sdk form)' },
  ]

  return (
    <div className="dim">
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline' }}>
        <span>│ </span>
        <span className="k" style={{ marginRight: '1ch' }}>format:</span>
        {items.map((item) => (
          <details
            key={item.key}
            open={openFormat === item.key}
            onToggle={(e) => {
              if ((e.target as HTMLDetailsElement).open) setOpenFormat(item.key)
              else if (openFormat === item.key) setOpenFormat(null)
            }}
            style={{ marginRight: 0 }}
          >
            <summary>{item.label}</summary>
            <pre style={{ gridColumn: '1 / -1', marginTop: '0.25rem' }}>{item.available ? item.render() : '(unavailable for this view)'}</pre>
          </details>
        ))}
      </div>
    </div>
  )
}
