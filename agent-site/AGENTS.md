# AGENTS.md — how to consume this surface

If you're an agent reading this, hi. Here is the minimum you need.

## Discovery

```http
GET /.well-known/afs-agent-manifest.json
```

Returns the full callable surface: every endpoint, every header, every media type, every event stream filter param,
every auth profile. Cache it.

## Auth

```http
POST /handshake
Authorization: Bearer <token>
Content-Type: application/json

{ "profile": "workspace-rw" }
```

Response is a `Capability` descriptor. It lists `granted: string[]` — exactly the tools you can call. If a tool isn't in
there, calling it returns 403 with a `Link` header pointing to `/why/:action_id` for the machine-readable reason.

## Conventions

- Every list endpoint returns `{ items: T[], etag, ... }`.
- Every record carries `_actions: [{verb, href, method, idempotent, schema?}]`. Discover next steps from there. Don't
  hardcode URLs from this readme — read them off the response.
- Every mutation accepts `Idempotency-Key: <uuid>`. Same key = same receipt.
- Every mutation accepts `X-AFS-DryRun: 1`. Returns the receipt that *would* have been produced, without applying.
- Every response carries `X-AFS-Cost: tokens=N,bytes=N,ms=N`. Cumulative at `/ledger`.
- Every list accepts `?at=<iso-ts>` to time-travel.
- Every URL accepts `?format=json|jsonl|curl|mcp|cli|py|ts` for snippet rendering.
- Concurrency: `ETag` on responses, `If-Match` on mutations. No locks.
- Errors include `Link: </why/:action_id>; rel="why"`.

## Streaming

```http
GET /activity
Accept: text/event-stream
```

One event per line. Filter with `?workspace=`, `?agent=`, `?session=`, `?since=<iso-ts>`. The `/replay` endpoint is just
`/activity?since=...` — there's no separate route.

## Receipts

Every successful mutation returns a `Receipt`:

```json
{
  "hash": "sha256:...",
  "action_id": "act-...",
  "ts": "2026-05-01T11:02:12.001Z",
  "verb": "file_write",
  "resource": "/workspaces/.../files/...",
  "before_ref": "checkpoint:cp-pp-011",
  "after_ref": "working-copy@...",
  "bytes_delta": 412,
  "cost": { "tokens": 0, "bytes": 412, "ms": 7 },
  "undo_token": "undo-...",
  "replay": { "curl": "...", "cli": "...", "mcp": "..." },
  "signature": "ed25519:..."
}
```

`POST /actions/undo` with the `undo_token` reverses it. Idempotent.

## Tools index

```http
GET /tools
GET /tools?id=file_write   # one tool with full schema and example
```

Filter by `?family=workspace|checkpoint|file|admin` or `?scope=read|write|admin`.

## What this surface does NOT have

- Login pages, signup pages, password reset, email verification, captchas.
- Modals, dialogs, multi-step wizards, "Step 1 of 2" interstitials.
- Tooltips, hover-preloads, animations, toasts.
- Light/dark theme. There is one theme: dim monospace.
- Sidebar nav. The top nav is six links and one tagline.

If you need any of those, the human UI lives at `ui/` in source. This isn't that.

## File organization (for the curious)

- `src/routes/*.tsx` — one file per route, default-export a component.
- `src/loaders.ts` — pure functions from params → data. Today they read mocks. Tomorrow they hit `/v1/...`. Call sites do not change.
- `src/snippets.ts` — `curl()`, `mcp()`, `cli()`, `py()`, `ts()`, `json()`, `jsonl()` — same data, every form.
- `src/frame.tsx` — page chrome. The format toggle uses `<details>` so it works without JS.
- `src/tokens.css` — the design system. CSS variables. One file. Don't grow a `components/` folder.
