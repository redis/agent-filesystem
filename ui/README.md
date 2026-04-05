# AFS Web UI

This package hosts the Agent Filesystem Web UI.

It intentionally reuses the shell frame from `re-multi-cluster-manager`:

- TanStack Router + Vite
- Redis UI theme/bootstrap
- shared sidebar and page title frame

The AFS product surfaces inside that frame are custom:

- workspace catalog and lifecycle actions
- checkpoint inspection and restore controls
- browser/editor workspace studio

Current state:

- frontend-only demo backed by a local in-browser store
- Redis Cloud alignment through Redis UI primitives and layout shell
- clean seams for replacing the demo store with real AFS APIs later

## Commands

```bash
npm install
npm run dev
npm run build
npm run test
```
