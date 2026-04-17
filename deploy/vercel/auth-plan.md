# AFS Cloud Auth Plan

Date: 2026-04-17
Status: Proposed next milestone

## Where we are now

The hosted service currently has browser-mediated CLI onboarding, but it does
not yet have a real website account system.

Today, the browser flow is doing two things:

- creating or reusing the default `getting-started` workspace
- handing the CLI a short-lived onboarding token so `afs up` can work

That is good enough to prove the hosted runtime, but it is not the final
product shape.

## What "real auth" means for AFS Cloud

The website needs:

- user sign-up and sign-in
- durable user records in Postgres
- cookie-backed web sessions
- workspace ownership and access control
- a protected UI and protected API routes
- CLI handoff that depends on a signed-in browser session, not an anonymous
  bootstrap page

## Recommended provider

Use Clerk as the first real hosted auth provider.

Why:

- Vercel's current authentication guidance recommends using a provider instead
  of building auth by hand.
- Clerk is a native Vercel Marketplace integration.
- Clerk gives us sign-up, sign-in, sessions, organizations, and polished hosted
  components without forcing us to invent the entire account system first.
- AFS Cloud is developer-facing SaaS, so organizations and membership models are
  useful early.

## Why not Sign in with Vercel as the main auth system

Sign in with Vercel is interesting, and we may want it later as an optional
social login for developer convenience.

It is not the best primary auth layer for v1 because:

- it requires every user to already have a Vercel account
- we still need our own user/org/workspace authorization model
- the product should not depend on Vercel identity long term if hosting moves
  away from Vercel

## Phase plan

### Phase 1: Website auth

- install Clerk on the Vercel project
- add sign-up and sign-in pages in the hosted UI
- create the local `user`, `organization`, and membership records we need in
  Postgres
- require an authenticated session for `/workspaces`, `/agents`, and related UI
  routes
- require authenticated identity for mutating control-plane API calls

### Phase 2: Workspace ownership

- associate each workspace/database record with the owning account or org
- scope workspace listings to the signed-in user/org
- enforce authorization in the control-plane API, not only in the UI

### Phase 3: CLI handoff

- keep `afs auth` as the entrypoint
- if the browser is not signed in, the user signs in first
- after sign-in, the browser attaches the CLI to the selected workspace
- exchange the one-time handoff into durable local CLI config

### Phase 4: Optional convenience providers

- optionally add Sign in with Vercel as a social login
- optionally add GitHub/google auth if useful

## Practical timing

This should be the next major build slice after the current onboarding polish.

Recommended order:

1. add real website auth
2. enforce workspace ownership
3. reconnect the CLI handoff through the authenticated session
4. then refine billing, invites, and multi-user collaboration
