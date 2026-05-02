// actions-row.tsx — two patterns for rendering an Action[].
//
// ActionsList   (detail pages): vertical, always expanded, full $ afs annotated form.
// ActionsInline (table rows):   compact inline verbs + "more (N) ›" toggle.

import { Fragment, useState } from 'react'
import { Link } from 'react-router-dom'
import type { Action } from './types'

// ────────────────────────────────────────────── full list (detail pages)

export function ActionsList({ actions }: { actions: Action[] }) {
  if (actions.length === 0) return null
  return (
    <ul className="flat actions-list">
      {actions.map((a) => (
        <li key={a.verb}>
          {a.cli ? (
            <span className="verb">$ {a.cli.join(' ')}</span>
          ) : (
            <>
              <span className="verb">{a.method}</span>{' '}
              <Link to={a.method === 'GET' ? a.href : '#'}>{a.href}</Link>
            </>
          )}
          {' '}<span className="chip">{a.idempotent ? 'idempotent' : 'non-idempotent'}</span>
          {a.cli && <span className="dim"> · {a.method} {a.href}</span>}
        </li>
      ))}
    </ul>
  )
}

// ────────────────────────────────────────────── compact (table cells)

export function ActionsInline({ actions, featuredCount = 3 }: { actions: Action[]; featuredCount?: number }) {
  const [expanded, setExpanded] = useState(false)
  if (actions.length === 0) return null

  const featured = actions.slice(0, featuredCount)
  const rest = actions.slice(featuredCount)
  const visible = expanded ? actions : featured

  return (
    <span className="actions-inline">
      {visible.map((a, i) => (
        <Fragment key={a.verb}>
          {i > 0 && ' '}
          <ActionVerb a={a} />
        </Fragment>
      ))}
      {rest.length > 0 && (
        <>
          {' '}
          <button
            type="button"
            className="more-toggle"
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
            aria-expanded={expanded}
          >
            {expanded ? 'less' : `more (${rest.length})`}
          </button>
        </>
      )}
    </span>
  )
}

function ActionVerb({ a }: { a: Action }) {
  // GET verbs go to their natural href (e.g. diff page).
  // non-GET verbs aren't browser-navigable; route them to the resource's
  // owner page (the workspace detail) so the click does something useful.
  const target = a.method === 'GET'
    ? a.href
    : (a.href.match(/^(\/workspaces\/[^/]+)/)?.[1] ?? a.href)
  return <Link className="verb" to={target}>{a.verb}</Link>
}
