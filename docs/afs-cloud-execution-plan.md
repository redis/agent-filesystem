# AFS Cloud Execution Plan

Date: 2026-04-16
Status: Draft for review

## Purpose

Turn the cloud-control-plane design into a reviewable execution sequence that
matches the codebase as it exists today.

This plan is intentionally narrower than the full design in
`docs/afs-cloud-control-plane-design.md`.
It focuses on the next implementation slices required to move from today's
local/self-hosted product to a real hosted cloud product.

## Repo Reality Check

The repository already contains meaningful cloud-enabling groundwork:

- explicit CLI product modes: `local`, `self-hosted`, and `cloud`
- a backend seam in `cmd/afs` instead of one hard-coded direct-Redis path
- an HTTP control plane with separate admin and client route groups
- self-hosted sync startup via control-plane-issued workspace session bundles
- a SQLite control-plane catalog for workspace routing and database registry
- opaque catalog-owned workspace ids already flowing through API responses
- a session catalog plus connected-agent views in the UI
- workspace-first UI navigation and route parameters

The key gap is that these pieces currently stop at `self-hosted`.
The hosted `cloud` mode is still intentionally unimplemented in the CLI, and
there is no hosted identity, token storage, secret store, or managed database
workflow yet.

## What This Plan Assumes

- Redis remains the live data plane for the first hosted release.
- sync mode ships before mount mode in real cloud mode.
- browser login for humans and short-lived workspace session bundles for local
  clients are separate layers.
- the web UI must participate in the same hosted identity system as the CLI;
  browser sign-in is a release requirement, not a later enhancement
- Redis Cloud browser-assisted linking is not required for the first hosted
  release.
- "external reachable database" ships before "external hybrid connector".

## Deployment And Tenancy Guardrails

- `self-hosted` and `cloud` should remain product modes of the same
  control-plane codebase and binary, with mode-specific bootstrap behavior,
  auth expectations, route exposure, and secret-store requirements
- the browser/admin and client/session surfaces may share one process or binary
  initially, but the implementation should preserve the option to split them
  into separate deployments or at least separate middleware stacks later
- the hosted `cloud` product should be treated as multi-tenant from the start:
  users belong to organizations, and workspaces, sessions, database bindings,
  and secrets are organization-scoped resources
- the first `self-hosted` rollout should remain single-tenant by default, using
  one implicit organization for the deployment rather than leaving org fields
  blank
- service-layer authorization must enforce tenant boundaries; route middleware
  alone is not sufficient

## Current Blocking Gaps

The codebase is closest to a hosted launch in these areas:

1. Identity is missing.
The control plane has no user/org model, no auth middleware, no PKCE flow, and
the CLI has no token/profile storage.

2. Cloud session issuance is missing.
The self-hosted session path exists, but the hosted equivalent does not yet
attach auth, policy, org ownership, or renewable short-lived credentials.

3. Secret-backed provider credentials are missing.
Database credentials still live in the local catalog/config shape instead of a
dedicated secret-store abstraction.

4. Hosted database workflows are missing.
The UI can manage registered databases for self-hosted mode, but not cloud
database bindings, provisioning jobs, or attachment validation.

5. Managed mount remains deferred.
Sync has the better architecture for a first WAN-facing release; mount should
follow only after sync is stable in hosted mode.

## Recommended Execution Order

## Phase A: Cloud identity foundation

Goal:
Introduce the minimum hosted identity model needed for browser login and CLI
login without changing the Redis data plane yet.

Scope:

- add control-plane user, organization, membership, and session models
- add auth middleware for browser/admin and client/session surfaces
- add a principal model that flows through control-plane service methods
- add service-layer authorization rules for workspaces, database bindings, and
  session issuance
- add browser session handling for the web UI
- implement Authorization Code + PKCE for `afs auth login`
- add CLI token storage and profile selection
- keep tokens out of `afs.config.json`; use OS keychain/keyring with an
  encrypted-file fallback stored separately from config
- add `afs auth login`, `afs auth logout`, and `afs auth status`
- add `cloudBackend` bootstrap that requires authenticated control-plane access
- define the initial tenant model:
  - `cloud` is multi-tenant by organization
  - `self-hosted` defaults to one implicit organization per deployment
  - no long-term design should rely on blank `organization_id` values

Likely code areas:

- `cmd/afs/`
- `internal/controlplane/`
- `ui/src/`

Acceptance:

- a user can log in from the CLI without entering Redis credentials
- the browser UI can render signed-in state and current account context
- authenticated requests succeed against hosted-only routes
- unauthenticated requests fail cleanly on protected routes
- tenant ownership checks are enforced in service methods, not only in HTTP
  middleware

Suggested validation:

- `go test ./cmd/afs ./cmd/afs-control-plane ./internal/...`
- CLI e2e test for PKCE login against a local dev control plane
- UI auth state smoke test

## Phase B: Cloud-issued sync sessions

Goal:
Make `afs up --mode sync` work in `cloud` mode using short-lived
control-plane-issued workspace session bundles.

Scope:

- implement the real `cloud` CLI backend
- make workspace selection/use persist hosted workspace ids end to end
- add auth-aware `POST /v1/workspaces/{workspace_id}/sessions`
- issue short-lived Redis credentials or brokered session tokens
- define the child-daemon auth model for hosted mode:
  - how the daemon receives its bootstrap/session material
  - how it renews long-running access without interactive browser login
  - how renewal failure and session expiry are surfaced locally
- add renew/expiry handling for long-running sync daemons
- remove the need for long-lived Redis credentials in managed local config

Phase-order note:

- full cloud session issuance for managed and external database bindings depends
  on the secret-store and binding model in Phase C
- Phase B can still begin earlier if the first hosted runtime is explicitly
  scoped to a temporary single AFS-managed backing Redis environment, rather
  than per-binding credential brokerage

Likely code areas:

- `cmd/afs/backend.go`
- `cmd/afs/controlplane_http_client.go`
- `cmd/afs/managed_session.go`
- `cmd/afs/sync_*.go`
- `internal/controlplane/http.go`
- `internal/controlplane/service.go`

Acceptance:

- hosted login -> workspace select -> `afs up --mode sync` works end to end
- sync daemon heartbeats and renews through the hosted control plane
- local config no longer requires durable Redis credentials for hosted mode
- session expiry and reconnect behavior are covered by tests
- daemon renewal is non-interactive and does not depend on browser cookies

Suggested validation:

- `go test ./cmd/afs ./cmd/afs-control-plane ./internal/...`
- e2e: login, create/select workspace, sync up, heartbeat, renew, down

## Phase C: Secret store and database binding model

Goal:
Introduce a production-safe way to persist provider and database credentials
before adding hosted provisioning flows.

Scope:

- add a `SecretStore` abstraction to the control plane
- add a development implementation plus one real encrypted implementation
- move external database credentials behind secret references
- define cloud database binding records distinct from local database registry
- keep the workspace catalog as routing/index metadata, not secret storage

Likely code areas:

- `internal/controlplane/`
- control-plane config loading and startup validation

Acceptance:

- hosted deployments fail fast when secret storage is misconfigured
- provider/database credentials are no longer stored in plain catalog rows
- binding records reference secret ids instead of embedding secrets

Suggested validation:

- unit tests for read/write/delete/health behavior
- startup validation tests
- migration tests from plain config-backed development state

## Phase D: Hosted managed and external database workflows

Goal:
Let the hosted product create a managed database or attach a reachable external
database from the web UI.

Scope:

- add cloud-facing database binding APIs
- add external database validation flow
- add managed database provisioning job model
- add UI flows for:
  - create managed database
  - attach external database
  - create workspace against a chosen binding
- keep browser-assisted Redis Cloud linking explicitly out of scope

Likely code areas:

- `internal/controlplane/`
- `ui/src/routes/`
- `ui/src/foundation/api/afs.ts`

Acceptance:

- a user can create a workspace backed by a managed database
- a user can attach a reachable external database and create a workspace on it
- job success/failure states are visible in the UI
- the workspace catalog resolves hosted workspaces without hidden database
  selection

Suggested validation:

- API tests for binding create/update/delete and validation failures
- UI tests for creation flows
- e2e: create workspace from both managed and external-reachable paths

## Phase E: Cloud-connected mount and hybrid BYODB follow-on

Goal:
Extend hosted support after sync mode is stable.

Scope:

- make mount startup consume cloud-issued session bundles too
- add presence reporting for mount sessions
- only after that, decide whether to build the external-hybrid connector path

Why last:

- sync is the safer and more WAN-friendly hosted starting point
- hybrid BYODB adds control-channel and orchestration complexity that is not
  required for the first hosted release

Acceptance:

- hosted mount lifecycle works end to end
- connected-client views correctly distinguish sync vs mount sessions
- any connector work is reviewed as a separate follow-on design/spec

## First Reviewable Release Definition

The first version that should count as "AFS Cloud" is:

- browser sign-in works
- CLI sign-in works
- a user can create a workspace in the hosted UI
- the workspace is backed by either:
  - an AFS-managed Redis database, or
  - a directly reachable attached external database
- the user can run `afs up --mode sync` locally with no long-lived Redis
  credential in config
- the hosted UI shows connected agents for that workspace

The first hosted release does not need:

- mount mode
- browser-assisted Redis Cloud linking
- private-network hybrid connector support

## Recommended PR Breakdown

To keep review and stabilization manageable, the next work should land in
roughly this order:

1. Auth and profile model scaffolding in CLI + control plane
2. Protected auth/session endpoints plus PKCE login flow
3. Cloud backend for sync session bootstrap and renewal
4. Secret-store abstraction and config migration
5. Hosted database binding API and validation model
6. Managed/external database UI flows

## Review Questions

These are the highest-value decisions to settle before implementation starts:

1. Should the first hosted release support only one organization per CLI
   profile, or do we need multi-org switching immediately?
2. What should the first real short-lived Redis credential format be:
   username/password pair, ACL token, or another brokered session format?
3. What should the first production secret-store backend be?
4. Do we want one managed Redis database per workspace for the first release?
5. Do we want to ship external reachable databases in the same release as
   managed databases, or one release later?
6. Do we want `self-hosted` auth to stay optional single-tenant at first, or
   should self-hosted deployments be able to enable the full multi-user/org
   model immediately?
7. Which settings belong at the user, organization, and workspace levels for
   the first hosted release?

## Recommended Immediate Next Step

If we want the smallest path to a reviewable implementation PR, the next spec
to write should be the auth-and-session slice:

- hosted user/org model
- browser session model
- CLI PKCE flow
- token storage/profile shape in `cmd/afs`
- service-layer principal propagation and authorization rules
- protected workspace-session routes
- cloud session bundle schema and renewal rules
- self-hosted implicit-organization behavior for the same control-plane binary

That is the narrowest slice that converts today's self-hosted groundwork into
the beginning of a real hosted product.
