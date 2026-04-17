# AFS Vercel Deployment Shape

Date: 2026-04-16
Status: First deployment guardrails

## Purpose

Capture the repo and runtime shape we should use for the first real AFS Cloud
deployment on Vercel.

This document is intentionally practical:

- what Vercel should run
- what should stay out of the first deployment
- which repo assumptions are safe vs unsafe

## Recommended First Deployment Unit

Deploy the control plane as a single Go service from this repository.

That means the first hosted unit is:

- `cmd/afs-control-plane`
- serving the HTTP API
- serving the embedded UI from `internal/uistatic/dist`

This is the right first shape because the control plane already:

- exposes the browser/admin and client/session HTTP surfaces
- embeds and serves the UI
- owns workspace routing and session bootstrap behavior

It avoids introducing an unnecessary frontend/backend split before the hosted
runtime itself is stable.

## What Not To Use As The Vercel Entry Point

Do not treat the root `Dockerfile` as the Vercel deployment contract.

Reasons:

- Vercel does not deploy Docker images directly
- the Dockerfile is still useful for self-hosted/container workflows
- making Vercel depend on the Docker path would hide the real runtime
  requirements instead of clarifying them

## Repo Strategy

For the first Vercel-hosted release, keep one deployable product surface in
this repo:

- the Go control plane at the repo root module

Keep these as supporting build inputs, not standalone Vercel apps:

- `ui/` builds the static assets embedded into the control plane
- `cmd/afs/` remains the downloadable CLI, not the hosted service
- `module/` and `mount/` remain local/self-hosted components, not Vercel
  runtime targets

## Current Vercel Blocker

The current control plane stores its workspace/session/database catalog in a
local SQLite file.

That is acceptable for local and self-hosted development, but it is not the
right persistence model for a real Vercel deployment because the hosted
filesystem is not durable enough to act as the control-plane source of truth.

So the key deployment rule is:

- Redis can stay external for the data plane
- the control-plane catalog must move to a durable network store

## First-Step Architecture Decision

Treat the catalog as a storage boundary now.

Immediate consequence:

- SQLite stays as the default local implementation
- the codebase should stop assuming SQLite is the only possible catalog
  backend

This keeps the first Vercel work honest: we are preparing the real service
contract instead of only adding deployment cosmetics.

## Build Strategy

Short term:

- keep the UI embedded in the Go service
- keep `internal/uistatic/dist` as the asset directory the control plane serves
- use `deploy/vercel/preview.sh` to stage a temporary Vercel build root that
  preserves the repo-root Go module before preview deploys

Follow-up we will need:

- an explicit deployment build path that rebuilds `ui/` before the Go service
  is compiled, so hosted deploys do not depend on stale checked-in assets

## Runtime Expectations

A Vercel-ready control-plane runtime should be configured entirely from
environment variables plus durable backing services.

Minimum expected runtime inputs:

- control-plane listen port from `PORT`
- Redis connection from `AFS_REDIS_*`
- catalog backend selection from catalog-specific env/config

Current catalog env contract:

- local/self-hosted default: `AFS_CATALOG_DRIVER=sqlite`
- hosted default: `AFS_CATALOG_DRIVER=postgres`
- explicit DSN: `AFS_CATALOG_DSN`
- provider fallback DSNs: `POSTGRES_URL_NON_POOLING`, `POSTGRES_URL`, `DATABASE_URL`

## Near-Term Sequence

1. Make the control-plane catalog pluggable, with SQLite as the initial driver.
2. Add a durable hosted catalog backend suitable for Vercel.
3. Lock down the Vercel build/deploy path for the single control-plane service.
4. Only then add project-level Vercel deployment configuration.
