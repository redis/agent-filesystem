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
