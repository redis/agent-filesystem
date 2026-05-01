# afs · agent-site

A parallel, fresh-start product surface for AFS, redesigned for the consumer that uses it the most: **agents**.

The existing human UI (under `ui/`) is unchanged and keeps shipping. This is a separate, isolated site with its own
package.json, no shared components, and no compromises.

## Why

The human UI has sidebars, modals, multi-step wizards, "Step 1 of 2" interstitials, Clerk OAuth login, hover preloads,
Lucide icons, light/dark theme, color-coded byte deltas, and breadcrumbs. Lovely for humans.

Agents don't read pages. They consume data, follow links programmatically, retry on idempotency keys, batch calls, and
fail silently when nobody told them why. The agent-filesystem surface is the same product redesigned for that consumer.

## Manifesto

1. **One canonical JSON per resource. HTML is a view of it.** Content negotiation via `Accept`. `?format=curl|mcp|cli|py|ts`
   returns *code snippets that produce the same JSON*. Not parallel UIs.
2. **Every URL is a callable.** Real `<a href>`. No fake `<button>`-as-nav. Pages work with no JS.
3. **Manifest, not marketing.** `/` is `cat /etc/afs`. No hero, no welcome interstitial.
4. **Tokens in, capabilities out.** No login screen. `/handshake` is the entry. One POST.
5. **No modals, no wizards.** One call carries all params.
6. **HATEOAS-shaped responses.** Every record carries `_actions: [{verb, href, schema, idempotent}]`.
7. **Receipts on every mutation.** `action_id`, `hash`, `before/after` refs, `undo_token`, `replay_command`. At `/receipts/:hash`.
8. **Cost in headers.** `X-AFS-Cost: tokens=N,bytes=N,ms=N` on every response.
9. **Dry-run as a header.** `X-AFS-DryRun: 1`. No "plan" DSL.
10. **Optimistic concurrency, not locks.** `ETag` + `If-Match`. No lock manager.
11. **Why-rejected is machine-readable.** `/why/:action_id` returns `{reason, required_capability, suggested_fix, retry_after}`.
12. **Streaming first-class.** `/activity` and `/sessions` are SSE-shaped (simulated with `setInterval` over mock log).
13. **Time-travel by default.** `?at=<ts>` on every list. `/replay` is `/activity` with filters.
14. **Verbs only.** `create`, `fork`, `restore`, `mount`, `revoke`. Never "save," "submit," "click."
15. **Boring monospace.** Norvig-grade dense, not aerc-grade ASCII art. Two grays, one accent.
16. **No `components/` folder.** Shared anything is abstraction debt. One file per route.

## Routes

| Path | Role |
|---|---|
| `/` | Manifest. Capabilities + auth state + every URL listed. |
| `/handshake` | Token in → capability descriptor out. Replaces login + signup + `/capabilities`. |
| `/workspaces` | List. Each row carries `_actions`. |
| `/workspaces/:id` | Single page (no tabs): state digest, checkpoints, sessions, recent activity, every action. |
| `/checkpoints/:id` | Detail + parent chain + diff link to any other ref. |
| `/activity` | Live SSE-style tail. JSONL via `?format=jsonl`. `?since=&agent=` makes it `/replay`. |
| `/sessions` | Live agent sessions, killable. |
| `/tools` | Every MCP+HTTP tool. `?id=<name>` filters to one with try-it form. |
| `/receipts/:hash` | Verifiable receipt for any state change. |
| `/why/:action_id` | Machine-readable reason an action was rejected. |
| `/.well-known/afs-agent-manifest.json` | Static discovery doc. |

## Run

```bash
cd agent-site
npm install
npm run dev
```

Vite picks port 5174 (5173 is taken by the human UI under `ui/`). Open `http://localhost:5174`.

```bash
npm run build  # static dist/
```

## Stack

- Vite + React + TypeScript
- `react-router-dom` for client-side routing (real `<a href>` fallback when JS is off)
- Plain CSS via `tokens.css` — no styled-components, no Tailwind, no Redis UI components
- Mock data via `src/mocks/*.json` — no fetch round-trip; loaders are sync ESM imports
- No Clerk, no Lucide, no animations, no shadows, no rounded corners

## Layout

```
agent-site/
  package.json
  vite.config.ts
  tsconfig.json
  index.html
  README.md          ← this file
  AGENTS.md          ← how an agent consumes this surface
  public/
    .well-known/
      afs-agent-manifest.json   ← discovery doc, runtime-served
  src/
    main.tsx         ← router setup
    tokens.css       ← THE design system
    types.ts         ← Workspace, Checkpoint, Session, Receipt, Tool…
    manifest.ts      ← typed mirror of the well-known manifest
    snippets.ts      ← data → curl/mcp/cli/py/ts string renderers
    loaders.ts       ← mock-data fetchers (one per resource)
    frame.tsx        ← page chrome (path header, format toggle, footer)
    mocks/           ← *.json fixtures, no factories
    routes/          ← one file per route, default-exports a component
```

## Status

Prototype. Mock data. Real backend integration is the obvious next step (the loaders are the only thing that has to
change). The point of this repo is the **surface and shape**, not the integration.
