# Cloud Architecture

Last reviewed: 2026-05-02.

This is the current cloud/control-plane architecture. It should describe what
exists now and the few important gaps. Detailed implementation history belongs
in git history, not in this doc.

## Product Modes

AFS supports three explicit modes:

- `local`: the CLI talks directly to Redis.
- `self-hosted`: the CLI talks to a private AFS control plane.
- `cloud`: the CLI talks to AFS Cloud and uses browser-mediated login.

`Self-managed` is the user-facing label for the private control-plane-backed
mode. The config value is still `self-hosted`.

## Control Plane

The control plane owns:

- auth runtime configuration
- workspace and checkpoint HTTP APIs
- workspace/session/catalog routing
- database registry and health
- onboarding tokens
- CLI access tokens
- MCP access tokens
- UI/admin/client route surfaces

Redis remains the data plane for workspace contents: manifests, blobs,
checkpoints, live roots, activity, and file history.

## Catalog

The control-plane catalog is the source of truth for discovery and routing:

- registered databases
- workspace-to-database routing
- workspace ownership metadata
- session catalog records
- onboarding tokens
- CLI and MCP token records
- database health and workspace refresh metadata

SQLite is the default local/self-managed catalog. Hosted deployments can use
Postgres with `AFS_CATALOG_DRIVER=postgres`.

## Auth

Implemented auth paths:

- `none` for local development
- `trusted-header` for deployments behind an auth proxy
- Clerk-backed browser auth
- CLI access tokens
- MCP access tokens
- browser-mediated CLI onboarding token exchange

Open auth work:

- full OAuth/PKCE profile and refresh-token model in the CLI
- OS-backed token/profile storage
- complete organization and membership model
- continued service-layer authorization audit as new tenant-scoped operations
  are added

## Database Bindings

Implemented:

- external reachable Redis databases can be registered through the UI/API
- registered database profiles store endpoint, username, password, DB index,
  TLS flag, owner, default selection, health, and stats
- workspace operations can route through registered database profiles

Not implemented:

- AFS-managed Redis provisioning
- production secret-store-backed credential storage
- explicit external database validation and credential rotation UX
- private BYODB connector where customer Redis is not reachable from cloud

## Local Runtime

Implemented:

- managed `afs ws mount` starts sync from a control-plane-issued workspace
  session bundle
- managed sync daemons heartbeat and disconnect through the control plane
- workspace detail, agents, overview, activity, and history views carry enough
  database/workspace routing context to operate without a global database
  selector

Open:

- cloud session renewal and credential rotation for long-running daemons
- cloud-connected FUSE/NFS mount mode
- private BYODB connector support

## Keep In Mind

- The browser/admin surface and client/session surface share code today, but
  should remain logically separate.
- Presence comes from session heartbeats, not activity logs.
- The catalog is routing metadata. Workspace contents remain in Redis.
- External database credentials should move behind a secret-store boundary
  before managed provisioning or enterprise BYODB hardening.
