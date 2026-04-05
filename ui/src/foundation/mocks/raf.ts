import type {
  RAFActivityEvent,
  RAFFile,
  RAFSavepoint,
  RAFState,
  RAFWorkspace,
} from "../types/raf";

function bytesLabel(files: RAFFile[]) {
  const totalBytes = files.reduce(
    (sum, file) => sum + new TextEncoder().encode(file.content).length,
    0,
  );

  if (totalBytes > 1024 * 1024) {
    return `${(totalBytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  return `${Math.max(1, Math.round(totalBytes / 1024))} KB`;
}

function buildSavepoint(
  id: string,
  name: string,
  createdAt: string,
  author: string,
  note: string,
  files: RAFFile[],
): RAFSavepoint {
  return {
    id,
    name,
    author,
    createdAt,
    note,
    fileCount: files.length,
    sizeLabel: bytesLabel(files),
    filesSnapshot: files.map((file) => ({ ...file })),
  };
}

function buildActivity(
  id: string,
  createdAt: string,
  actor: string,
  kind: string,
  scope: string,
  title: string,
  detail: string,
): RAFActivityEvent {
  return {
    id,
    actor,
    createdAt,
    detail,
    kind,
    scope,
    title,
  };
}

const paymentsCurrentFiles: RAFFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T10:40:00Z",
    content: `# Payments Workspace

This workspace is validating browser editing and checkpoint restore behavior.

- active checkpoint: before-refactor
- editor draft: dirty
- next milestone: cloud control-plane hookup`,
  },
  {
    path: "src/app.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:44:00Z",
    content: `export function App() {
  return {
    workspace: "payments-portal",
    mode: "editor-pass",
    checkpoint: "before-refactor"
  };
}`,
  },
  {
    path: "src/routes/editor.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:48:00Z",
    content: `export const editorRoute = {
  id: "payments-editor",
  state: "dirty"
};`,
  },
];

const paymentsBaselineFiles: RAFFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T09:05:00Z",
    content: `# Payments Workspace

This workspace tracks the checkout hardening effort for Redis Cloud hosted agents.

- active checkpoint: baseline-ui
- next milestone: browser editing`,
  },
  {
    path: "src/app.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T09:12:00Z",
    content: `export function App() {
  return {
    workspace: "payments-portal",
    mode: "baseline"
  };
}`,
  },
];

const memoryFiles: RAFFile[] = [
  {
    path: "memories/customer-a.md",
    language: "markdown",
    modifiedAt: "2026-04-02T16:08:00Z",
    content: `# Customer A

- prefers weekly digests
- rollout window after 18:00 UTC`,
  },
  {
    path: "memories/customer-b.md",
    language: "markdown",
    modifiedAt: "2026-04-02T16:11:00Z",
    content: `# Customer B

- import review requested
- editor permissions pending`,
  },
];

const sandboxFiles: RAFFile[] = [
  {
    path: "sandbox.yaml",
    language: "yaml",
    modifiedAt: "2026-04-01T12:00:00Z",
    content: `runtime: docker
workspace: support-sandbox
allow:
  - shell
  - editor
  - checkpoints`,
  },
  {
    path: "scripts/bootstrap.sh",
    language: "shell",
    modifiedAt: "2026-04-01T12:03:00Z",
    content: `#!/usr/bin/env bash
set -euo pipefail
echo "Preparing support sandbox"`,
  },
];

const paymentsSavepoints = [
  buildSavepoint(
    "sp-payments-before-refactor",
    "before-refactor",
    "2026-04-03T10:36:00Z",
    "maya",
    "Captured a recovery point before the editor refactor.",
    paymentsBaselineFiles,
  ),
  buildSavepoint(
    "sp-payments-baseline-ui",
    "baseline-ui",
    "2026-04-03T09:22:00Z",
    "rafa",
    "Prepared the workspace for Redis Cloud Web UI review.",
    paymentsBaselineFiles,
  ),
];

const memorySavepoints = [
  buildSavepoint(
    "sp-memory-initial",
    "initial",
    "2026-04-02T16:15:00Z",
    "anika",
    "Imported customer memory workspace.",
    memoryFiles,
  ),
];

const sandboxSavepoints = [
  buildSavepoint(
    "sp-sandbox-initial",
    "initial",
    "2026-04-01T12:05:00Z",
    "sam",
    "Created support sandbox template.",
    sandboxFiles,
  ),
];

const paymentsWorkspace: RAFWorkspace = {
  id: "payments-portal",
  name: "payments-portal",
  description: "Checkout debugging workspace mirrored into AFS for browser editing and checkpoint recovery.",
  cloudAccount: "Redis Cloud / Customer Success",
  databaseId: "db-payments-portal",
  databaseName: "payments-portal-us-east-1",
  redisKey: "afs:payments-portal",
  region: "us-east-1",
  mountedPath: "~/.afs/workspaces/payments-portal",
  status: "healthy",
  source: "git-import",
  createdAt: "2026-04-02T18:15:00Z",
  updatedAt: "2026-04-03T10:48:00Z",
  draftState: "dirty",
  headSavepointId: "sp-payments-before-refactor",
  tags: ["production", "frontend", "cloud"],
  files: paymentsCurrentFiles,
  savepoints: paymentsSavepoints,
  activity: [
    buildActivity(
      "evt-payments-1",
      "2026-04-03T10:48:00Z",
      "maya",
      "file.updated",
      "file",
      "Edited src/routes/editor.tsx",
      "Saved draft changes inside the Web UI editor.",
    ),
    buildActivity(
      "evt-payments-2",
      "2026-04-03T10:36:00Z",
      "maya",
      "savepoint.created",
      "savepoint",
      "Created before-refactor",
      "Captured a recovery point before the refactor.",
    ),
  ],
};

const memoryWorkspace: RAFWorkspace = {
  id: "customer-memory",
  name: "customer-memory",
  description: "Shared memory workspace for support and solution engineering follow-ups.",
  cloudAccount: "Redis Cloud / Solutions",
  databaseId: "db-customer-memory",
  databaseName: "customer-memory-eu-west-1",
  redisKey: "afs:customer-memory",
  region: "eu-west-1",
  mountedPath: "~/.afs/workspaces/customer-memory",
  status: "syncing",
  source: "cloud-import",
  createdAt: "2026-04-02T16:15:00Z",
  updatedAt: "2026-04-02T16:15:00Z",
  draftState: "clean",
  headSavepointId: "sp-memory-initial",
  tags: ["memory", "shared"],
  files: memoryFiles,
  savepoints: memorySavepoints,
  activity: [
    buildActivity(
      "evt-memory-1",
      "2026-04-02T16:15:00Z",
      "anika",
      "workspace.imported",
      "workspace",
      "Imported customer-memory",
      "Workspace imported from a managed Redis Cloud source.",
    ),
  ],
};

const sandboxWorkspace: RAFWorkspace = {
  id: "support-sandbox",
  name: "support-sandbox",
  description: "Blank workspace for reproducing customer issues with isolated checkpoints.",
  cloudAccount: "Redis Cloud / Support",
  databaseId: "db-support-sandbox",
  databaseName: "support-sandbox-us-central-1",
  redisKey: "afs:support-sandbox",
  region: "us-central-1",
  mountedPath: "~/.afs/workspaces/support-sandbox",
  status: "attention",
  source: "blank",
  createdAt: "2026-04-01T12:05:00Z",
  updatedAt: "2026-04-01T12:05:00Z",
  draftState: "clean",
  headSavepointId: "sp-sandbox-initial",
  tags: ["sandbox", "ops"],
  files: sandboxFiles,
  savepoints: sandboxSavepoints,
  activity: [
    buildActivity(
      "evt-sandbox-1",
      "2026-04-01T12:05:00Z",
      "sam",
      "workspace.created",
      "workspace",
      "Created support-sandbox",
      "Blank workspace provisioned for support workflows.",
    ),
  ],
};

export const initialRAFState: RAFState = {
  workspaces: [paymentsWorkspace, memoryWorkspace, sandboxWorkspace],
};

export function cloneInitialRAFState() {
  return JSON.parse(JSON.stringify(initialRAFState)) as RAFState;
}
