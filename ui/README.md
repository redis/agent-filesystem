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

- demo-by-default with optional local HTTP control plane mode
- Redis Cloud alignment through Redis UI primitives and layout shell
- clean seams for switching between the demo store and real AFS APIs

## Commands

```bash
npm install
npm run dev
npm run build
npm run test
```

Because the UI depends on private `@redislabsdev/*` packages hosted on GitHub Packages,
you must configure npm auth before `npm install` will work:

```bash
cd ui
cp .npmrc.example .npmrc
export NPM_AUTH_TOKEN=YOUR_TOKEN
npm install
```

The token must be able to read the `redislabsdev` GitHub Packages scope.

To run the UI against the local AFS control plane from the repo root:

```bash
./afs setup   # once, if afs.config.json does not exist yet
make web-dev
```

Or run just the UI against an already-running control plane:

```bash
VITE_AFS_API_BASE_URL=http://127.0.0.1:8091 npm run dev
```
