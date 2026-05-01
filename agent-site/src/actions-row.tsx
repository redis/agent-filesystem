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
          {i > 0 && <span className="sep"> · </span>}
          <ActionVerb a={a} />
        </Fragment>
      ))}
      {rest.length > 0 && (
        <>
          <span className="sep"> · </span>
          <button
            type="button"
            className="more-toggle"
            onClick={(e) => {
              e.stopPropagation()
              setExpanded(!expanded)
            }}
            aria-expanded={expanded}
          >
            {expanded ? '‹' : `more (${rest.length}) ›`}
          </button>
        </>
      )}
    </span>
  )
}

function ActionVerb({ a }: { a: Action }) {
  if (a.method === 'GET') {
    return <Link className="verb" to={a.href}>{a.verb}</Link>
  }
  return <span className="verb">{a.verb}</span>
}
