# AFS Hosted Onboarding Flows

Date: 2026-04-17
Status: In progress

## Goal

Support two entry ramps into the same hosted product:

1. CLI-first onboarding
2. Web-first onboarding

Both ramps should converge on the same steady state:

- authenticated AFS account
- first filesystem/workspace created
- authenticated CLI installed locally
- normal `afs` workflow available immediately

## Current Implementation Snapshot

As of 2026-04-17, the hosted production deploy now has these pieces in place:

- Vercel preview deploys boot from [main.go](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/main.go)
- control-plane catalog uses Neon/Postgres in hosted mode
- workspace data plane uses Redis Cloud via `REDIS_URL`
- first-run hosted bootstrap auto-seeds a managed database profile when the
  catalog is empty
- `afs auth` / `afs login` can launch a browser flow automatically
- `/connect-cli` can create or reuse `getting-started` and hand the CLI back a
  short-lived onboarding token
- `afs auth login [--control-plane-url <url>] [--workspace <workspace>]`
  exchanges that browser bootstrap into local CLI config
- `afs auth logout` and `afs auth status` exist

This means the hosted service can now support the initial "run `afs auth`,
finish browser handoff, then run `afs up`" flow.

## Product Modes

The CLI should present three user-facing choices:

- `Local`
- `Self Managed`
- `Cloud Managed`

These are product labels.

Internally, the current config/runtime names remain:

- `local`
- `self-hosted`
- `cloud`

## Shared Product Rule

Do not create two different hosted products.

CLI-first and web-first onboarding should both land on the same hosted control
plane, account model, workspace model, and CLI auth/session flow.

## Flow A: CLI-First

The developer:

1. clones the repo
2. runs `make`
3. runs `./afs setup`
4. chooses `Cloud Managed`, `Self Managed`, or `Local`

### Local

- CLI configures direct Redis/local behavior
- no browser login required

### Self Managed

- CLI asks for the control-plane URL
- CLI connects to the user-owned control plane
- normal workspace selection/bootstrap follows

### Cloud Managed

- CLI opens the hosted browser flow
- hosted flow creates or reuses `getting-started`
- browser hands the CLI a short-lived onboarding token through the localhost
  callback
- CLI stores its durable local config and then continues with `afs up`

Near-term improvement:

- make `afs setup` kick directly into the browser flow after `Cloud Managed` is
  selected
- add real hosted account auth and web sessions ahead of workspace access

## Flow B: Web-First

The developer:

1. visits `agentfilesystem.vercel.com`
2. signs in or creates an account
3. lands in the default `getting-started` workspace
4. downloads the CLI from the workspace page
5. runs `afs auth login` or `afs login`
6. finishes the browser handoff
7. runs `afs up`
8. starts using AFS

### Important rule

Do not ship a special binary with long-lived auth baked into it.

Instead:

- ship the normal signed CLI artifact
- use browser login and a short-lived bootstrap token to attach the CLI
- let the CLI store its own long-lived local auth state afterward

## Naming

Externally, the hosted product may talk about a "filesystem" because that is
the product concept.

Internally, the codebase still uses "workspace" heavily.

Short-term rule:

- product copy may say "filesystem"
- API/runtime internals may continue using "workspace"

Long-term, we can decide whether to fully rename the hosted surface or keep
"filesystem" as marketing language over a workspace-first runtime model.

## Implemented Hosted Capabilities

- downloadable CLI artifacts from `/v1/cli`
- `getting-started` default onboarding in the hosted UI
- browser-first CLI auth handoff via `afs auth`
- hosted control plane bootstrapped against Neon + Redis Cloud

## Remaining Hosted Capabilities

To support both entry ramps cleanly, the next missing pieces are:

- real browser sign-up/sign-in and account model
- hosted session identity beyond one-time CLI bootstrap
- direct browser launch from `afs setup`
- smoother first-run `afs up` UX for cloud mode
- production-domain auth, onboarding, and smoke coverage

## Auth Milestone

The current hosted flow is browser-mediated bootstrap, not real account auth.

That was intentional: first prove deploy/build/runtime/storage on Vercel, then
add the actual user account layer on top.

The next major milestone is:

1. website sign-up/sign-in
2. durable user + organization records in Postgres
3. cookie-backed web sessions
4. workspace ownership and authorization checks
5. browser-authenticated CLI handoff

See [auth-plan.md](/Users/rowantrollope/git/agent-filesystem/deploy/vercel/auth-plan.md)
for the recommended hosted auth direction.

Important deployment rule:

- `local` and `self-hosted` must not require a cloud auth provider
- `cloud managed` can use a hosted provider through the pluggable auth boundary

## Near-Term Build Order

1. keep preview deploys healthy and repeatable on Vercel
2. add real hosted account auth
3. make the CLI/browser handoff consume the authenticated web session
4. make cloud-mode `afs up` feel first-class after login
5. add durable end-to-end coverage on the public production domain
