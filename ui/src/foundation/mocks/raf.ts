import type {
  RAFActivityEvent,
  RAFFile,
  RAFSavepoint,
  RAFSession,
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

function buildSession(
  session: Omit<RAFSession, "savepoints" | "headSavepointId"> & {
    savepoints: RAFSavepoint[];
    headSavepointName: string;
  },
): RAFSession {
  const [firstSavepoint] = session.savepoints;
  const currentSavepoint = session.savepoints.find(
    (savepoint) => savepoint.name === session.headSavepointName,
  );

  return {
    ...session,
    headSavepointId: currentSavepoint == null ? firstSavepoint.id : currentSavepoint.id,
    savepoints: session.savepoints,
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

const paymentsMainFiles: RAFFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T09:05:00Z",
    content: `# Payments Workspace

This workspace tracks the checkout hardening effort for Redis Cloud hosted agents.

- default session: main
- active savepoint: baseline-ui
- next milestone: browser-based session import`,
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
  {
    path: "notes/session-plan.md",
    language: "markdown",
    modifiedAt: "2026-04-03T09:20:00Z",
    content: `## Session plan

1. Import repository into RAF
2. Fork a review session
3. Save checkpoints before risky changes`,
  },
];

const paymentsFixFiles: RAFFile[] = [
  {
    path: "README.md",
    language: "markdown",
    modifiedAt: "2026-04-03T10:40:00Z",
    content: `# Payments Workspace

Fix-login session is validating session import and rollback behavior.

- checkpoint before refactor
- file browser wired
- editor draft dirty`,
  },
  {
    path: "src/app.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:44:00Z",
    content: `export function App() {
  return {
    workspace: "payments-portal",
    mode: "fix-login",
    savepoint: "before-refactor"
  };
}`,
  },
  {
    path: "src/routes/session.tsx",
    language: "typescript",
    modifiedAt: "2026-04-03T10:48:00Z",
    content: `export const sessionRoute = {
  id: "fix-login",
  status: "dirty"
};`,
  },
];

const memoryMainFiles: RAFFile[] = [
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

- session import requested
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
  - session-import`,
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

const paymentsMainSavepoints = [
  buildSavepoint(
    "sp-payments-initial",
    "initial",
    "2026-04-02T18:15:00Z",
    "rafa",
    "Imported baseline checkout repo into RAF.",
    paymentsMainFiles,
  ),
  buildSavepoint(
    "sp-payments-baseline-ui",
    "baseline-ui",
    "2026-04-03T09:22:00Z",
    "rafa",
    "Prepared the workspace for Redis Cloud Web UI review.",
    paymentsMainFiles,
  ),
];

const paymentsFixSavepoints = [
  buildSavepoint(
    "sp-fix-login-forked",
    "forked-from-main",
    "2026-04-03T10:15:00Z",
    "maya",
    "Forked from main after import review.",
    paymentsMainFiles,
  ),
  buildSavepoint(
    "sp-fix-login-before-refactor",
    "before-refactor",
    "2026-04-03T10:36:00Z",
    "maya",
    "Captured session state before editor refactor.",
    paymentsFixFiles,
  ),
];

const memorySavepoints = [
  buildSavepoint(
    "sp-memory-initial",
    "initial",
    "2026-04-02T16:15:00Z",
    "anika",
    "Imported customer memory workspace.",
    memoryMainFiles,
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
  description: "Checkout debugging workspace mirrored into RAF for collaborative agent sessions.",
  cloudAccount: "Redis Cloud / Customer Success",
  databaseId: "db-payments-portal",
  databaseName: "payments-portal-us-east-1",
  redisKey: "raf:payments-portal",
  region: "us-east-1",
  mountedPath: "~/.raf/workspaces/payments-portal",
  status: "healthy",
  source: "git-import",
  createdAt: "2026-04-02T18:15:00Z",
  updatedAt: "2026-04-03T10:48:00Z",
  defaultSessionId: "session-main",
  tags: ["production", "frontend", "cloud"],
  sessions: [
    buildSession({
      id: "session-main",
      name: "main",
      description: "Canonical saved branch for checkout UI exploration.",
      author: "rafa",
      createdAt: "2026-04-02T18:15:00Z",
      updatedAt: "2026-04-03T09:22:00Z",
      lastRunAt: "2026-04-03T09:24:00Z",
      status: "clean",
      kind: "main",
      files: paymentsMainFiles,
      savepoints: paymentsMainSavepoints,
      headSavepointName: "baseline-ui",
    }),
    buildSession({
      id: "session-fix-login",
      name: "fix-login",
      description: "Scratch session validating the browser editor flow.",
      author: "maya",
      createdAt: "2026-04-03T10:15:00Z",
      updatedAt: "2026-04-03T10:48:00Z",
      lastRunAt: "2026-04-03T10:42:00Z",
      status: "dirty",
      kind: "branch",
      files: paymentsFixFiles,
      savepoints: paymentsFixSavepoints,
      headSavepointName: "before-refactor",
    }),
  ],
  activity: [
    buildActivity(
      "evt-payments-1",
      "2026-04-03T10:48:00Z",
      "maya",
      "file.updated",
      "session",
      "Edited src/routes/session.tsx",
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
    buildActivity(
      "evt-payments-3",
      "2026-04-03T10:15:00Z",
      "maya",
      "session.forked",
      "session",
      "Forked fix-login",
      "New branch session created from main.",
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
  redisKey: "raf:customer-memory",
  region: "eu-west-1",
  mountedPath: "~/.raf/workspaces/customer-memory",
  status: "syncing",
  source: "cloud-import",
  createdAt: "2026-04-02T16:15:00Z",
  updatedAt: "2026-04-02T16:15:00Z",
  defaultSessionId: "session-memory-main",
  tags: ["memory", "shared"],
  sessions: [
    buildSession({
      id: "session-memory-main",
      name: "main",
      description: "Primary memory lane with review-only access.",
      author: "anika",
      createdAt: "2026-04-02T16:15:00Z",
      updatedAt: "2026-04-02T16:15:00Z",
      lastRunAt: "2026-04-02T16:18:00Z",
      status: "clean",
      kind: "imported",
      files: memoryMainFiles,
      savepoints: memorySavepoints,
      headSavepointName: "initial",
    }),
  ],
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
  description: "Blank workspace for reproducing customer issues with isolated agent sessions.",
  cloudAccount: "Redis Cloud / Support",
  databaseId: "db-support-sandbox",
  databaseName: "support-sandbox-us-central-1",
  redisKey: "raf:support-sandbox",
  region: "us-central-1",
  mountedPath: "~/.raf/workspaces/support-sandbox",
  status: "attention",
  source: "blank",
  createdAt: "2026-04-01T12:05:00Z",
  updatedAt: "2026-04-01T12:05:00Z",
  defaultSessionId: "session-sandbox-main",
  tags: ["sandbox", "ops"],
  sessions: [
    buildSession({
      id: "session-sandbox-main",
      name: "main",
      description: "Template sandbox used to spin up short-lived sessions.",
      author: "sam",
      createdAt: "2026-04-01T12:05:00Z",
      updatedAt: "2026-04-01T12:05:00Z",
      lastRunAt: "2026-04-01T12:07:00Z",
      status: "clean",
      kind: "main",
      files: sandboxFiles,
      savepoints: sandboxSavepoints,
      headSavepointName: "initial",
    }),
  ],
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
