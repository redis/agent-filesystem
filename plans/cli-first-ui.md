# CLI-First UI Repositioning

Status: active
Owner: rowan
Created: 2026-05-01
Updated: 2026-05-01

## Goal

Reposition `ui/` from a feature-complete CRUD console into an *observability surface* that makes the CLI/MCP feel like the protagonist of AFS. Today's UI signals "infrastructure-with-a-console" (AWS / Atlas / managed service); we want users to feel "developer-first agent file infrastructure where my terminal is the actor and this page is the viewport."

References: Stripe Dashboard (rich but secondary to the API), PlanetScale (observability + branch ops), Datadog (almost zero "create" actions).

## Scope

**In scope** — repositioning `ui/` in place; harvesting useful pieces from `agent-site/`; landing the contracts (receipts, `/why`, ETag, X-AFS-Cost, dry-run, SSE) in `internal/controlplane/`; rewriting marketing copy at `afs.cloud/`.

**Out of scope** — redesigning the CLI itself; redesigning MCP tools; mobile-friendly layout; renaming `afs`; building a separate ops/admin surface (admin pages stay where they are).

## Strategy

- **Branch** `cli-first-ui` off `main`. Edit `ui/` in place. Reuse Clerk auth, TanStack Router, React Query, Redis UI components, theme tokens.
- **Side-by-side review**: `git worktree add ../afs-main main` to keep current `ui/` running alongside the branch checkout.
- **Harvest** from `agent-site/`: format-toggle component, snippet renderers, JSON-everywhere ethos, contracts spec.
- **Sunset** `agent-site/` after harvest — move to `plans/archive/agent-site/` or delete.

## Checklist

### Phase 1 — Branch + harvest (1–2 days)

- [ ] Create `cli-first-ui` branch off `main`
- [ ] Set up worktree workflow doc (`README` or `plans/cli-first-ui.md`)
- [ ] Move `agent-site/src/snippets.ts` and the format-toggle out of `frame.tsx` into `ui/src/lib/snippets.ts` and `ui/src/components/format-toggle.tsx`
- [ ] Verify both old `ui/` (worktree) and new `ui/` (branch) build cleanly

### Phase 2 — Remove mutation flows (3–4 days)

- [ ] Audit every `<Dialog>`, `<DialogCard>`, `<DialogOverlay>` in `ui/src/components/`
- [ ] Remove "Create Workspace" toolbar button from `workspaces.tsx`. Replace with copyable `afs ws create <name>` snippet panel.
- [ ] Remove "Edit" pencil column on the workspaces table. Edit settings becomes a manual-override disclosure on detail page.
- [ ] Remove "Delete" row action. Move behind a "manual override" disclosure on detail.
- [ ] Remove `GettingStartedOnboardingDialog` multi-step. Replace with inline first-run hero: "your CLI hasn't done anything yet — install it and run `afs ws create my-repo`."
- [ ] Remove `agents_.add.tsx` route (agents connect via MCP/CLI, never UI)
- [ ] Audit `ui/src/components/free-tier-limit-dialog.tsx`, `mcp-connection-panel.tsx`, `access-token-empty-state.tsx`, `getting-started-onboarding-dialog.tsx` — keep only what survives the new model

### Phase 3 — Rebuild as observability-first (1 week)

- [ ] New default landing at `/inspect` (rename Dashboard → Inspector). Live activity stream as primary content.
- [ ] Live "your CLI just did X" toasts in the top bar — driven by SSE on `/v1/activity` filtered to current user
- [ ] Per-page "do this from your terminal" panel — small mono block with copyable `afs ...` form. Lives at the bottom of every detail page.
- [ ] Status header: "5 workspaces · 3 active sessions · 12 ops/min · ↑ live" — always visible, always updating
- [ ] Sidebar reorder: Activity → Workspaces → Sessions → Tools → Settings (drop Overview entirely)
- [ ] Workspace detail: "recent activity" + "live sessions" front and center; settings/danger-zone behind disclosure

### Phase 4 — Marketing landing (3–4 days)

- [ ] `afs.cloud/` (anonymous) becomes a marketing page with terminal-shaped hero (`afs ws create my-repo` typing demo). Primary CTA: install CLI. Secondary: open inspector.
- [ ] Logged-in default goes to `/inspect`, not `/`

### Phase 5 — Backend contracts (parallel, 2–3 weeks)

These ship into `internal/controlplane/` independently. The inspector tolerates their absence (graceful degradation).

- [ ] Receipts on changelog stream: extend [internal/controlplane/changelog.go](../internal/controlplane/changelog.go) with `signature: ed25519` + `undo_token`. ~1 week.
- [ ] `/why/:action_id` — new Redis stream `afs:rejections:<workspace>` with `{reason, required_capability, current_capability, suggested_fix, retry_after}`. ~3 days.
- [ ] ETag/If-Match middleware on every resource; 412 on conflict with structured remediation. ~1 week.
- [ ] `X-AFS-Cost: tokens=N,bytes=N,ms=N` middleware. ~3 days.
- [ ] `X-AFS-DryRun: 1` short-circuit (touches every mutation handler). ~1 week.
- [ ] SSE on `/v1/activity` and `/v1/sessions`. ~3 days each.
- [ ] `POST /v1/auth/exchange` for same-origin Clerk → token bridge. ~2 days.

### Phase 4 — Embedded Console (abandoned)

Tried to ship a Redis-Insight-style embedded terminal on the Inspector page (built and removed 2026-05-02). Strict-CLI-only commands, ghost-text autocomplete, plain-text output. Output was technically correct but the result didn't earn its place — felt like a half-shell rather than the protagonist. Removed in full.

If we revisit: rather than partial-shell-in-the-browser, lean into the real CLI from a dedicated route (e.g. an inline-rendered `xterm.js` connected to a websocket-backed PTY running on the control plane in a sandbox). That would be a real terminal, not an emulation. Not on the current branch.

### Phase 5 — Backend contracts (parallel, 2–3 weeks)

The contracts surfaced by `agent-site/` need to land in `internal/controlplane/`. These are independent of the UI repositioning but make the inspector + console more useful as they ship:

- Receipts on changelog stream, `/why/:action_id`, ETag/If-Match middleware, X-AFS-Cost middleware, X-AFS-DryRun, SSE on `/v1/activity`, `POST /v1/auth/exchange`. (See earlier plan for details.)

### Phase 6 — Cutover (3–4 days)

- [ ] Final demo of new `ui/` vs old `ui/` (worktree) — confirm CLI-first feel
- [ ] Merge `cli-first-ui` branch
- [ ] Move `agent-site/` to `plans/archive/agent-site-sketch/` or delete
- [ ] Audit `ui/` for any unused components/dialogs
- [ ] Update `README.md`, `AGENTS.md`, `docs/`

## In Flight

- Phase 1, not yet started

## Decisions / Blockers

- **Branch over parallel directory** — decided 2026-05-01. Reasons: avoid duplicated Clerk/router/query plumbing; avoid maintaining 3 UIs during transition; PRs review as refactor diffs; cutover is just `git merge` not a directory rename. Side-by-side need is met by `git worktree add ../afs-main main`.
- **Inspector path** — leaning `/inspect` over `/dashboard` or `/`. Verb-y, observability-honest. Decide before Phase 4.
- **First-run experience** — leaning toward "your CLI hasn't done anything yet" hero (no auto-provisioned starter workspace) to reinforce CLI-first story on day one. Open question.
- **Light theme** — keep light/dark toggle (observability tools live in mixed lighting). Don't force dark-only.
- **`agent-site/` final disposition** — move to `plans/archive/` after harvest, vs delete entirely. Lean toward archive (small footprint, useful as a frozen reference).

## Verification

- [ ] **Phase 3 internal demo**: side-by-side old `ui/` (worktree) vs new `ui/` (branch). New version's home is the live activity stream; no "Create Workspace" button on table; every detail page has a copyable CLI snippet panel.
- [ ] **Phase 5 contract test**: every mutation in the new UI produces a real signed receipt; clicking a 4xx surfaces a `/why` entry; ETag conflict produces a 412 with structured remediation.
- [ ] **End-to-end story test**: an outside dev opens `afs.cloud/` cold. Lands on a CLI-centric narrative. Installs the CLI. Runs `afs ws create demo`. Opens the inspector tab. Sees their command's effects animate in. Mental model: "my terminal made that happen; this page is watching."
- [ ] No regressions on settings, admin, login/signup flows.

## Result

Fill in before archiving.
