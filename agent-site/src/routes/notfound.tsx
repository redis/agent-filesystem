import { Link, useLocation } from 'react-router-dom'
import { Frame } from '../frame'

export default function NotFound() {
  const loc = useLocation()
  return (
    <Frame
      path={loc.pathname}
      realPath={loc.pathname}
      meta="404 — no route matches"
      request={{ method: 'GET', path: loc.pathname }}
      json={{ error: 'not_found', path: loc.pathname }}
    >
      <h1>404</h1>
      <p className="err">no route at <code>{loc.pathname}</code>.</p>
      <p>
        manifest at <Link to="/">/</Link>. discovery doc at{' '}
        <a href="/.well-known/afs-agent-manifest.json"><code>/.well-known/afs-agent-manifest.json</code></a>.
      </p>
    </Frame>
  )
}
