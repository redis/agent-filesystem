# Template install cleanup (current)

Current decision: templates use `Template.agentSkill` as a client-neutral source
of truth. The default install path is direct MCP registration plus a
workspace-named skill:

- Claude Code: `claude mcp add ...` plus `~/.claude/skills/afs-<workspace>/SKILL.md`
- Codex: `~/.codex/config.toml` plus `~/.agents/skills/afs-<workspace>/SKILL.md`
- Generic clients: raw MCP JSON plus copyable agent instructions

The Claude Code plugin zip remains an advanced bundle path, not the primary
template UX. Historical notes below may still mention the earlier
`claudePlugin` field and plugin-first approach.

# Claude Code plugin download from template

## Goal
After workspace creation from template, offer **"Download Claude Code plugin"** — a zip with MCP config + auto-triggering skill + slash commands, pre-filled with the workspace's bearer token. User runs `claude plugin install <zip>`; their agent auto-uses the workspace without explicit invocation.

## Approach
Client-side zip gen via JSZip. Extend `Template` with `claudePlugin` field (skill description + body + commands). Generator emits file tree; UI zips + triggers download. No backend work (token already returned at create time). Ship for `shared-agent-memory` template first.

## Tasks

### Template data (`ui/src/features/templates/templates-data.ts`)
- [ ] Extend `Template` type: `claudePlugin?: { skillDescription: string; skillBody: string; commands?: Array<{ name: string; body: string }> }`
- [ ] Fill `claudePlugin` for `shared-agent-memory`:
  - `skillDescription`: sharp auto-trigger — mentions afs-shared-memory MCP + the grep-first / record-durable-facts behavior
  - `skillBody`: AGENTS.md content, lightly rewritten to reference the actual MCP tools (`mcp__afs-shared-memory__file_grep`, etc.)
  - `commands`: `/memory-search <query>` and `/memory-record <title>` (explicit escape hatches)

### Generator (`ui/src/features/templates/claude-plugin.ts` — new)
- [ ] `buildClaudePlugin(template, workspaceName, controlPlaneUrl, token) → Array<{path, content}>`
- [ ] Emits:
  - `.claude-plugin/plugin.json` — name `afs-<workspaceName>`, version, description
  - `.mcp.json` — hosted HTTP shape with inlined bearer
  - `skills/<slug>/SKILL.md` — frontmatter with `skillDescription` + body
  - `commands/<name>.md` — one per command
  - `README.md` — install cmd + token-rotation note

### UI (`CreateWorkspaceDialog.tsx`)
- [ ] Add JSZip: `npm i jszip` in `ui/`
- [ ] New `SuccessSection` above current "Point your agent" section, shown only when `template.claudePlugin` exists:
  - Heading "Install as a Claude Code plugin"
  - Body: "Your agent will auto-use this workspace whenever relevant — no setup prompt needed."
  - Download button → builds tree, zips, triggers blob download as `afs-<workspaceName>.zip`
  - Code block showing `claude plugin install ~/Downloads/afs-<name>.zip`
- [ ] Keep existing raw-MCP-config section as fallback (for Codex/Cursor/other clients)

### Verify
- [ ] Build passes; download produces a well-formed zip
- [ ] Unzip inspection: files match expected layout, token inlined, JSON valid
- [ ] `claude plugin install` against the zip succeeds; `/mcp` shows `afs-<name>` connected
- [ ] Ask a casual non-trivial question — confirm skill triggers without explicit mention
- [ ] Record a durable fact — confirm it writes a new entry + appends to index.md

## Unresolved
- Skill description wording — I'll draft; want review before shipping.
- Commands worth it, or does the skill alone cover it? Leaning: ship both; commands cost ~200 lines and give explicit user control.
- Plugin README scope — one-paragraph install note, or full token-rotation / multi-workspace story?
- Where does the download button sit relative to the existing MCP config block — above (plugin first, raw config fallback) or below?

## Review
All resolved by user: (1) trust iteration post-install, (2) ship both skill + commands, (3) short README, (4) plugin section above raw MCP config.

**Files changed:**
- `ui/package.json`, `ui/package-lock.json` — added `jszip@^3.10.1`
- `ui/src/features/templates/claude-plugin.ts` — NEW: `buildClaudePlugin()` generator
- `ui/src/features/templates/claude-plugin.test.ts` — NEW: 7 unit tests, all green
- `ui/src/features/templates/templates-data.ts` — added `TemplateClaudePlugin` type, `claudePlugin` field on `Template`, `sharedMemoryClaudePlugin` spec wired onto `shared-agent-memory`
- `ui/src/features/workspaces/CreateWorkspaceDialog.tsx` — imported JSZip + generator, added `downloadClaudePlugin()` helper, inserted plugin-install `SuccessSection` above raw MCP config (only shown when `template.claudePlugin` present)

**Verification:**
- `npx tsc --noEmit` clean
- `npm run build` 616 ms, succeeds
- `npx vitest run` — 20/20 pass (including 7 new)
- Lint: 12 errors pre-change = 12 errors post-change (all pre-existing in unrelated lines)

**Manual verification still needed:**
- Create shared-memory workspace, click "Download plugin", inspect zip contents
- Install flow succeeds; `/mcp` shows server connected
- Non-trivial question auto-triggers the skill without explicit mention
- `/memory-search` and `/memory-record` slash commands fire as expected

## Fix: marketplace wrapper (post-install feedback)
First attempt failed: `claude plugin install <zip>` only accepts marketplace names, not local zips.

**Fix:** zip ships a minimal local marketplace. New layout:
\`\`\`
afs-<workspace>/                       (single top-level dir; Safari unzips here)
├── .claude-plugin/marketplace.json    (catalogs the plugin)
├── plugins/afs-<workspace>/
│   ├── .claude-plugin/plugin.json
│   ├── .mcp.json
│   ├── skills/shared-memory/SKILL.md
│   └── commands/*.md
└── README.md
\`\`\`

**New install flow (two commands inside Claude Code):**
\`\`\`
/plugin marketplace add ~/Downloads/afs-<workspace>
/plugin install afs-<workspace>@afs-<workspace>
\`\`\`

**Changed:** `claude-plugin.ts` emits new layout + `installCommands()` helper. `claude-plugin.test.ts` updated (9/9 pass). Dialog shows both install commands in the code block.

## Fix 2: /plugin gate — add install.sh fallback
Second feedback: user's Claude Code refused with `/plugin isn't available in this environment`. Plugin feature is gated (version or env-specific). Need a universal fallback.

**Fix:** bundle now ships an `install.sh` at the bundle root that:
- Copies `skills/<slug>/` into `~/.claude/skills/`
- Copies each `commands/*.md` into `~/.claude/commands/`
- Runs `claude mcp remove --scope user <name>` then `claude mcp add --scope user --transport http <name> <url> --header "Authorization: Bearer <token>"` (idempotent)
- Prints a warning + manual config snippet if `claude` CLI is absent

**Zip perms:** `install.sh` now marked 0o755 via JSZip `unixPermissions` + `platform: "UNIX"` so it's executable on extract (no manual `chmod` needed).

**README** lists both install paths: Option A (`/plugin` marketplace), Option B (`bash install.sh`).

**UI:** success dialog shows the `/plugin` block as primary plus a hint line with the fallback command, so users whose `/plugin` is gated see the escape hatch immediately.

**Tests:** 10/10 pass, including install.sh content check.

---

# Self-hosted agent onboarding

## Goal
`Connect a new agent` in self-hosted mode: zero-config install, no extra login step. Cloud flow unchanged.

## Approach
Install script (served from self-hosted origin) auto-runs `afs config set` after binary drop, seeding `control-plane-url` + `product-mode`. UI learns mode via `/v1/auth/config` and branches guide.

## Tasks

### Backend
- [ ] Extend `install_script.go` template: inject `ProductMode` alongside `BaseURL`. After binary write, shell calls `"$INSTALL_DIR/afs" config set config.source <mode>` and `"$INSTALL_DIR/afs" config set controlPlane.url <baseURL>` when mode != cloud. Cloud keeps current behavior (`afs login` handles it).
- [ ] Add `product_mode` field to `authRuntimeConfigResponse` in `internal/controlplane/auth.go`. Populate from same source install script uses.

### UI
- [ ] Read `productMode` from the config endpoint (already fetched for auth). Pass into `AgentSetupGuide`.
- [ ] In `AgentSetupGuide.tsx`, branch curl panel:
  - cloud → current 2-step flow (install + `afs login`)
  - self-hosted/local → 1 step (install only); hint: "Run `afs up` to connect"
- [ ] MCP tab: drop `afs login` mention for non-cloud.

### Verify
- [ ] Unit test install script template renders self-hosted config command.
- [ ] Unit test `authRuntimeConfigResponse` includes product mode.
- [ ] Manually run install.sh against local control plane → confirm `afs.config.json` has correct URL + mode.
- [ ] UI: toggle server mode, confirm guide swaps.

## Unresolved questions
- Self-hosted identity later → keep config shape (`product-mode`, `control-plane-url`) stable so adding `auth-token` later is additive. OK?
- Should local mode (redis-only, no control plane) show a different message entirely ("no setup needed, run `afs up`")? Or same as self-hosted?
- Installer runs `afs config set` — if user already has a config pointing at a different server, overwrite silently or prompt? Suggest overwrite (they just ran an install from that server).

---

# Control-plane MCP + token-model redesign + agent plugins

## Goal
Ship (a) a control-plane MCP alongside the existing workspace MCP — one endpoint, one tool catalog, scope-filtered by token; (b) a redesigned UI token model (no more "create an MCP server"); (c) root-level Claude Code + Codex plugins teaching base AFS usage; (d) template-specific use-case skills that layer on top. Killer onboarding: user pastes one control-plane token, agent issues workspace tokens on demand.

## Architecture (locked)

**One endpoint**: `<base>/mcp`. **One tool catalog**:
- file: `file_read`, `file_write`, `file_grep`, `file_list`, `file_glob`, `file_replace`, `file_insert`, `file_delete_lines`, `file_patch`, `file_lines`
- workspace: `workspace_list`, `workspace_get`, `workspace_create`, `workspace_fork`, `workspace_delete`
- checkpoint: `checkpoint_list`, `checkpoint_create`, `checkpoint_restore`
- token: `mcp_token_issue`, `mcp_token_revoke`

**Scope filters `tools/list`**:
- `workspace:<name>` token → file tools (bound to workspace, no `workspace` param) + checkpoint tools for that workspace.
- `control-plane` token → workspace tools + token tools + cross-workspace checkpoint_list. **No file tools.**

**Token prefixes**: `afs_cp_…` (control-plane) · `afs_mcp_…` (workspace). Single catalog table, new `scope` column.

**Server names in agent config**: `afs` (control-plane) · `afs-<workspace>` (workspace). Same URL, different `Authorization` header.

## Phases

### Phase 1 — Research ✅ DONE

**Claude Code — key findings:**
- `userConfig` is real. Fields declared in `plugin.json` get prompted at install time. `${user_config.KEY}` substitution works in `.mcp.json`, hooks, commands. Sensitive values go to keychain; non-sensitive exported as `CLAUDE_PLUGIN_OPTION_<KEY>`.
- Marketplace resolution: GitHub source looks for `.claude-plugin/marketplace.json` **at repo root only**. No subpath support. For subdirs, use `git-subdir` source type in the marketplace entry itself.
- Skills namespace as `/plugin-name:skill-name`. So our skill becomes `/agent-filesystem:afs-connect`.
- Desktop: no `/plugin` UI, but plugins installed via CLI load and function in desktop — matches prior understanding.
- Skill frontmatter supports `allowed-tools`, `model`, `effort`, `paths` (file-glob gate), `context: fork` (subagent isolation). Not essential for v1 but good to know.
- `.claude-plugin/` holds manifest only; `skills/`, `commands/`, `.mcp.json` at repo/plugin root.
- Dependencies array supported: `dependencies: [{name, version?}]` — use-case plugins can require base plugin.
- Canonical docs: https://code.claude.com/docs/en/plugins.md, plugins-reference.md, plugin-marketplaces.md, discover-plugins.md

**Codex — key findings:**
- Plugins are real and shipping (OpenAI's new Codex agent — app, CLI, IDE extension).
- Manifest: `.codex-plugin/plugin.json`. Fields: `name`, `version`, `description`, `skills` (path), `mcpServers` (path), `apps` (path), `interface{displayName, shortDescription, longDescription, developerName, category, capabilities[], websiteURL, privacyPolicyURL, termsOfServiceURL, defaultPrompt, brandColor, composerIcon, screenshots}`. Repo scaffold is the real format, not speculative.
- MCP config: TOML at `~/.codex/config.toml` under `[mcp_servers.<name>]`. Transports: stdio + Streamable HTTP (no SSE). HTTP works for AFS.
- Install: `/plugins` slash inside Codex or `codex marketplace add` — GitHub, git URL, local dir, marketplace.json URL.
- Built-in `@plugin-creator` skill scaffolds new plugins.
- Self-serve public publishing "coming soon"; user-added marketplaces from GitHub work today.
- Docs: https://developers.openai.com/codex/plugins, /codex/plugins/build, /codex/mcp

**Answers to outstanding questions:**
- **Q5 (Codex availability)**: yes, full plugin system exists; parallel structure viable.
- **Q7 (use-case plugin distribution)**: ship **both** paths. GitHub-marketplace with `userConfig` for public/generic install; UI-generated bespoke bundle for zero-prompt users coming from the workspace-creation flow.

### Phase 1.5 — Plan refinements from research
- Base Claude plugin uses `userConfig` declaratively (control_plane_url string, control_plane_token sensitive). `${user_config.*}` substitution in `.mcp.json` replaces the `/afs-connect` paste flow as the primary onboarding. `/afs-connect` remains as fallback for edge cases / self-hosted complications.
- Use-case plugins declare `dependencies: [{name: "agent-filesystem"}]` so base-installed-first is enforced.
- Codex base plugin mirrors structure: `.codex-plugin/plugin.json` + `.mcp.json` (HTTP transport) + `skills/`. Needs Codex-equivalent of userConfig (research suggests `apps` / `.app.json` for auth — verify during Phase 6).

### Phase 2 — Control-plane MCP (backend, Go)
- [ ] Extend token catalog: add `scope` column (`control-plane` | `workspace:<name>`). Migrate existing rows to `workspace:<name>`.
- [ ] New token issuance path for control-plane tokens (UI + internal). Prefix `afs_cp_`.
- [ ] Auth middleware: parse token, load scope, attach to request ctx.
- [ ] MCP server: consolidate to single `/mcp` endpoint. `tools/list` returns scope-filtered catalog.
- [ ] Tool handlers (new):
  - [ ] `workspace_list`, `workspace_get`, `workspace_create`, `workspace_fork`, `workspace_delete`
  - [ ] `checkpoint_list` (cross-workspace for control-plane, single for workspace), `checkpoint_create`, `checkpoint_restore`
  - [ ] `mcp_token_issue` (returns `{url, token, serverName}`), `mcp_token_revoke`
- [ ] Destructive ops (`workspace_delete`, `workspace_fork`, `checkpoint_restore`) gated behind control-plane scope only.
- [ ] Audit log on all control-plane tool calls (actor, action, target, timestamp).
- [ ] Tests: scope filtering, tool behavior per scope, token issuance round-trip.

### Phase 3 — UI token-model redesign
- [ ] Drop "Create MCP server" framing from workspace pages. Replace with "Access" tab listing tokens for that workspace.
- [ ] New page: `Settings > Agent access` (or top-level "Agent access") — control-plane token management. Hero card when none exist; collapsed row when present.
- [ ] Issue-token modal: for control-plane, just name + expiry; for workspace, name + profile + expiry.
- [ ] Success modal: tabbed copy-config (Claude Code CLI, Codex, raw JSON) — pre-assembled snippet per client.
- [ ] Global audit/admin view of all tokens (optional, nice-to-have).
- [ ] Remove legacy "MCP servers" nomenclature from all copy.

### Phase 4 — Claude Code root plugin (generic base)
- [ ] `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json` at repo root (refresh existing)
- [ ] `skills/afs/SKILL.md`: scope-agnostic — teaches one-endpoint-one-server-per-token model, tool catalog by scope, server-naming convention (`afs` / `afs-<workspace>`), concurrency rules, token profiles, the self-service onboarding pattern (agent calls `mcp_token_issue` to mint workspace tokens)
- [ ] `commands/afs-connect.md`: interactive — paste control-plane URL + control-plane token, run `claude mcp add` registering server name `afs`
- [ ] Root `README.md` section documenting install path (CLI first; desktop picks up after)

### Phase 5 — Template use-case plugins (Claude Code)
- [ ] Rewrite `templates/shared-memory/claude/redis-shared-memory/skills/shared-memory/SKILL.md`: pure workflow layer — assumes base plugin installed, references tools only, no re-teaching mechanics
- [ ] Add `/memory-setup` command: uses control-plane MCP (already connected) → `workspace_list` → user picks/creates → `mcp_token_issue` → `claude mcp add` registering `afs-shared-memory`
- [ ] Update template README: "install base first, then this; run `/memory-setup`"
- [ ] Port same pattern to other templates (coding-standards, team-board) — defer if Phase 4-6 running long

### Phase 6 — Codex plugins (parallel structure)
- [ ] Research-driven: base Codex plugin at `plugins/agent-filesystem/` (or equivalent path the docs prescribe)
- [ ] Per-template Codex use-case plugins at `templates/<name>/codex/` — flesh out existing `redis-shared-memory` stub
- [ ] Codex-equivalent of `/afs-connect` and `/memory-setup` flows (adapted to whatever interactive mechanism Codex skills support)

### Phase 7 — UI generator update
- [ ] `ui/src/features/templates/claude-plugin.ts` — generator emits new two-server config (base `afs` + bespoke `afs-<workspace>` with inlined token), plus the use-case skill. Zero manual paste for one-click users.
- [ ] Codex equivalent generator

### Phase 8 — Verification
- [ ] End-to-end: fresh machine → install base Claude plugin from GitHub → `/afs-connect` with control-plane token → install shared-memory plugin → `/memory-setup` → agent creates workspace, issues token, registers `afs-shared-memory`, can file_write into it
- [ ] Same flow for Codex
- [ ] Claude Code desktop picks up plugins installed via CLI
- [ ] Scope filter correctness: workspace token cannot call `workspace_create`; control-plane token cannot call `file_read`
- [ ] Audit log records every control-plane call

## Open questions (resolved or carried over)
1. ~~Endpoint path~~ — `<base>/mcp` (single, token scope decides tool surface)
2. ~~Token prefix~~ — `afs_cp_` / `afs_mcp_`
3. ~~`mcp_token_issue` return shape~~ — full `{url, token, serverName}` bundle
4. ~~Workspace creation perms~~ — control-plane token can create by default
5. **Codex availability** — resolve in Phase 1
6. ~~Destructive ops~~ — ship in v1, gated to control-plane scope
7. **Use-case plugin distribution** — resolve post-Phase-1 (leaning: both marketplace-listed + UI-generated)
8. ~~Always-on control-plane MCP~~ — yes, auth-gated, prominent UI
9. ~~UI token model~~ — single catalog, scope column, no "MCP servers" framing

## Next
Start Phase 1 research. Report findings in-chat before Phase 2 kicks off.

---

# Revised phase order (post-research)

Phase 1 done. All plugin scaffolding from this session **deleted** to start clean once MCP is right. UI generator (`ui/src/features/templates/claude-plugin.ts`) left intact — pre-existing shipping code; revisit in Phase 4+.

**New order:**

### Phase 2A — UI token-model cleanup (current focus)
Clean up the "MCP server" framing before anything else. No control-plane concept yet — just rename + simplify so the new mental model lands on a clean shell.

- [ ] Audit UI: every surface that says "MCP server"/"MCP access token"/etc. List each file with the current copy and the new copy.
- [ ] Workspace detail page: "MCP servers" section → "Access tokens" section. Same underlying list, renamed columns, no functional change.
- [ ] `CreateWorkspaceDialog` success screen: retire "MCP server config" framing. Present as "Your first access token" + existing copy-config block.
- [ ] Global tokens view (if exists): rename to "Access tokens".
- [ ] Remove any dead copy referencing the old model.
- [ ] No backend changes.

### Phase 2B — Backend: scope column + user-scoped issuance endpoint
- [ ] Schema migration: add `scope` column (`workspace:<name>` | `control-plane`). Backfill existing rows with `workspace:<name>` derived from their workspace binding.
- [ ] Token format: `afs_cp_{id}_{secret}` prefix for control-plane tokens (parallel to existing `afs_mcp_`).
- [ ] New route: `POST /v1/mcp-tokens` — user-scoped (no workspace binding) issuance. Returns same shape + `scope=control-plane`. Auth via existing user session.
- [ ] Corresponding `GET`/`DELETE` routes for listing/revoking user-scoped tokens.
- [ ] Audit-log plumbing for MCP token issuance (new stream or append to existing). Actor, action, scope, timestamp.
- [ ] Tests mirror `http_test.go` pattern.

### Phase 2C — Backend: tools + scope filter
- [ ] Extend `AuthIdentity` / request context to carry `scope`.
- [ ] `tools/list` filters by scope: workspace token sees file + single-workspace checkpoint tools; control-plane token sees workspace mgmt + cross-workspace checkpoint list + token mgmt tools.
- [ ] New tool handlers:
  - [ ] `workspace_list`, `workspace_get`, `workspace_create`, `workspace_fork`, `workspace_delete` (delegate to `DatabaseManager` methods)
  - [ ] `mcp_token_issue` (returns `{url, token, serverName}`), `mcp_token_revoke`
  - [ ] `checkpoint_list` cross-workspace variant for control-plane scope
- [ ] Destructive ops (`workspace_delete`, `workspace_fork`, `checkpoint_restore`) gated to control-plane scope.
- [ ] Tests for scope gating + per-tool happy path.

### Phase 2D — UI: control-plane card
- [ ] New section at top of access-tokens page / settings: "Control-plane access token" — hero card when none exist, collapsed row when present.
- [ ] Issue modal + success modal with tabbed copy-config (Claude Code / Codex / raw JSON).
- [ ] Revoke flow + "revoke all control-plane access" admin button.

### Phase 3+ — Plugins (Claude Code + Codex)
Re-scoped from original plan. Picks up after backend is right. Use findings from Phase 1.

## Deleted this round
- `/.claude-plugin/` (root)
- `/commands/` (root)
- `/skills/afs/`
- `/templates/shared-memory/claude/` (entire subtree)

## Next
Phase 2A audit: enumerate every UI surface that uses old MCP-server framing, then present rename plan.
