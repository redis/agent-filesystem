import { docsTopicById, docsTopics } from "../docs/docs-topics";
import type { DocsTopic, DocsTopicId } from "../docs/docs-topics";
import { bottomNavigationItems, navigationItems, resolveNavigationTitleParts } from "../../layout/navigation-items";
import { publicNavItems, publicRepoLink } from "../../layout/public-navigation";
import { canonicalWorkspaceName, displayWorkspaceName } from "../../foundation/workspace-display";

const docsBaseHref = "https://github.com/redis/agent-filesystem/blob/main/docs";
const repoHref = publicRepoLink.href;

export type SiteAgentResource = {
  href: string;
  label: string;
};

export type SiteAgentDocument = {
  title: string;
  markdown: string;
  assetPath?: string;
  resources?: ReadonlyArray<SiteAgentResource>;
};

type SiteAgentDocumentOptions = {
  controlPlaneUrl: string;
  siteOrigin: string;
  search?: string;
};

type SiteAgentRouteContext = SiteAgentDocumentOptions & {
  pathname: string;
  searchParams: URLSearchParams;
};

const canonicalDocsByTopic: Partial<Record<DocsTopicId, SiteAgentResource>> = {
  "how-it-works": {
    label: "Guide: agent-filesystem.md",
    href: `${docsBaseHref}/guides/agent-filesystem.md`,
  },
  cli: {
    label: "Reference: cli.md",
    href: `${docsBaseHref}/reference/cli.md`,
  },
  workspaces: {
    label: "Guide: agent-filesystem.md",
    href: `${docsBaseHref}/guides/agent-filesystem.md`,
  },
  "local-files": {
    label: "Guide: agent-filesystem.md",
    href: `${docsBaseHref}/guides/agent-filesystem.md`,
  },
  "mcp-agents": {
    label: "Reference: mcp.md",
    href: `${docsBaseHref}/reference/mcp.md`,
  },
  "typescript-sdk": {
    label: "Reference: typescript.md",
    href: `${docsBaseHref}/reference/typescript.md`,
  },
  "python-sdk": {
    label: "Reference: python.md",
    href: `${docsBaseHref}/reference/python.md`,
  },
  "self-managed": {
    label: "Internals: cloud.md",
    href: `${docsBaseHref}/internals/cloud.md`,
  },
  performance: {
    label: "Internals: performance.md",
    href: `${docsBaseHref}/internals/performance.md`,
  },
  faq: {
    label: "Docs index",
    href: `${docsBaseHref}/README.md`,
  },
};

const topicQuickActions: Partial<Record<DocsTopicId, string[]>> = {
  cli: [
    "curl -fsSL \"{{CONTROL_PLANE}}/v1/cli?os=$(uname -s)&arch=$(uname -m)\" -o afs && chmod +x afs",
    "afs auth login",
    "afs ws mount getting-started ~/getting-started",
  ],
  workspaces: [
    "afs ws create my-project",
    "afs ws mount my-project ~/my-project",
    "afs cp create my-project before-risky-change",
  ],
  "local-files": [
    "afs ws mount getting-started ~/getting-started",
    "afs status",
    "afs ws unmount getting-started",
  ],
  "mcp-agents": [
    "afs mcp",
    "afs ws mount getting-started ~/getting-started",
  ],
  "typescript-sdk": [
    "npm install @agentfilesystem/sdk",
    "Use the TypeScript reference for create/mount/search/checkpoint flows.",
  ],
  "python-sdk": [
    "pip install agent-filesystem",
    "Use the Python reference for create/mount/search/checkpoint flows.",
  ],
  "self-managed": [
    "make web-dev",
    "afs config set config.source self-managed",
    "afs auth login --self-hosted http://localhost:8080",
  ],
};

export function getSiteAgentDocument(
  pathname: string,
  { controlPlaneUrl, siteOrigin, search = "" }: SiteAgentDocumentOptions,
): SiteAgentDocument {
  const normalizedPath = normalizePublicPath(pathname);
  const routeContext: SiteAgentRouteContext = {
    pathname: normalizedPath,
    controlPlaneUrl,
    siteOrigin,
    search,
    searchParams: new URLSearchParams(search),
  };

  if (normalizedPath === "/" || normalizedPath === "/home") {
    return buildHomeDocument(siteOrigin);
  }
  if (normalizedPath === "/docs") {
    return buildDocsIndexDocument(siteOrigin);
  }
  if (normalizedPath === "/downloads") {
    return buildDownloadsDocument(siteOrigin, controlPlaneUrl);
  }
  if (normalizedPath === "/agent-guide") {
    return {
      title: "Agent Guide",
      markdown: `${buildCommonHeader("Agent Guide", normalizedPath, siteOrigin)}

## Canonical Source

- Public markdown asset: [agent-guide.md](${joinUrl(siteOrigin, "/agent-guide.md")})
- Repo guide: [docs/guides/agent-filesystem.md](${docsBaseHref}/guides/agent-filesystem.md)

## Notes

- The full markdown guide is appended below.
- This is the most copy-friendly page on the site for handing AFS context to another agent.
`,
      assetPath: "/agent-guide.md",
      resources: [
        { label: "Public asset", href: joinUrl(siteOrigin, "/agent-guide.md") },
        { label: "Repo guide", href: `${docsBaseHref}/guides/agent-filesystem.md` },
      ],
    };
  }

  const topic = docsTopics.find((item) => item.path === normalizedPath);
  if (topic != null) {
    return buildTopicDocument(topic, siteOrigin, controlPlaneUrl);
  }

  const appDocument = buildAppDocument(routeContext);
  if (appDocument != null) {
    return appDocument;
  }

  return {
    title: "AFS public site",
    markdown: `${buildCommonHeader("AFS public site", normalizedPath, siteOrigin)}

## Available pages

${buildSiteMap(siteOrigin)}

## Fallback

- This route does not have a dedicated agent-facing markdown deck yet.
- Switch back to Human mode if you need the visual treatment for this page.
`,
    resources: buildBaseResources(),
  };
}

function buildHomeDocument(siteOrigin: string): SiteAgentDocument {
  return {
    title: "Agent Filesystem",
    markdown: `${buildCommonHeader("Agent Filesystem", "/", siteOrigin)}

## Summary

AFS gives agents a filesystem-shaped workspace backed by Redis.

- The CLI, web UI, MCP server, and SDKs all talk to the same live workspace model.
- Local edits change live state immediately.
- Checkpoints are explicit restore points, not automatic snapshots.
- Workspaces can be mounted to real directories so shells, editors, scripts, and agents can all use ordinary paths.

## Quick start

\`\`\`bash
curl -fsSL https://afs.cloud/install.sh | bash
afs auth login
afs ws mount getting-started ~/getting-started
\`\`\`

## Why agents care

- One workspace model across CLI, MCP, SDKs, and browser flows.
- Real files and real directories instead of hidden chat state.
- Forkable workspaces for parallel agent lines of work.
- Checkpoint and restore without inventing a separate persistence layer.

## Public links

- Docs: [${joinUrl(siteOrigin, "/docs")}](${joinUrl(siteOrigin, "/docs")})
- Downloads: [${joinUrl(siteOrigin, "/downloads")}](${joinUrl(siteOrigin, "/downloads")})
- Agent guide: [${joinUrl(siteOrigin, "/agent-guide")}](${joinUrl(siteOrigin, "/agent-guide")})
- Repo: [${repoHref}](${repoHref})
`,
    resources: [
      { label: "Docs", href: "/docs" },
      { label: "Downloads", href: "/downloads" },
      { label: "Agent Guide", href: "/agent-guide" },
      { label: "Repo", href: repoHref },
    ],
  };
}

function buildDocsIndexDocument(siteOrigin: string): SiteAgentDocument {
  const topicList = docsTopics
    .map((topic) => `- [${topic.title}](${joinUrl(siteOrigin, topic.path)}) - ${topic.summary}`)
    .join("\n");

  return {
    title: "Docs",
    markdown: `${buildCommonHeader("Docs", "/docs", siteOrigin)}

## Canonical markdown docs

- [docs/README.md](${docsBaseHref}/README.md)
- [docs/guides/agent-filesystem.md](${docsBaseHref}/guides/agent-filesystem.md)
- [docs/reference/cli.md](${docsBaseHref}/reference/cli.md)
- [docs/reference/mcp.md](${docsBaseHref}/reference/mcp.md)
- [docs/reference/typescript.md](${docsBaseHref}/reference/typescript.md)
- [docs/reference/python.md](${docsBaseHref}/reference/python.md)

## Topic pages

${topicList}

## Fast path

\`\`\`bash
afs auth login
afs ws mount getting-started ~/getting-started
afs mcp
\`\`\`

## Notes

- The website docs are a curated overview.
- The repo markdown files above are the canonical copy-paste sources for agents.
`,
    resources: [
      { label: "Docs README", href: `${docsBaseHref}/README.md` },
      { label: "Guide", href: `${docsBaseHref}/guides/agent-filesystem.md` },
      { label: "CLI ref", href: `${docsBaseHref}/reference/cli.md` },
      { label: "MCP ref", href: `${docsBaseHref}/reference/mcp.md` },
    ],
  };
}

function buildDownloadsDocument(siteOrigin: string, controlPlaneUrl: string): SiteAgentDocument {
  const downloadCmd = `curl -fsSL "${joinUrl(controlPlaneUrl, "/v1/cli?os=$(uname -s)&arch=$(uname -m)")}" -o afs && chmod +x afs`;

  return {
    title: "Downloads",
    markdown: `${buildCommonHeader("Downloads", "/downloads", siteOrigin)}

## Install the CLI

\`\`\`bash
${downloadCmd}
sudo mv afs /usr/local/bin/afs
afs auth login
\`\`\`

## After install

\`\`\`bash
afs ws mount getting-started ~/getting-started
afs status
\`\`\`

## Notes

- The download endpoint serves a binary matched to the current control plane version.
- Sync mode is the recommended first run because it gives agents real local directories.
`,
    resources: [
      { label: "Docs", href: "/docs" },
      { label: "CLI reference", href: `${docsBaseHref}/reference/cli.md` },
    ],
  };
}

function buildTopicDocument(topic: DocsTopic, siteOrigin: string, controlPlaneUrl: string): SiteAgentDocument {
  const canonical = canonicalDocsByTopic[topic.id];
  const sectionHeadings = topic.sections.map((section) => `- ${section.heading}`).join("\n");
  const related = topic.related
    .map((id) => {
      const relatedTopic = docsTopicById[id];
      return `- [${relatedTopic.title}](${joinUrl(siteOrigin, relatedTopic.path)}) - ${relatedTopic.summary}`;
    })
    .join("\n");
  const quickActions = topicQuickActions[topic.id];
  const quickActionBlock = quickActions == null
    ? ""
    : `
## Quick actions

\`\`\`text
${quickActions.map((line) => line.replace("{{CONTROL_PLANE}}", controlPlaneUrl)).join("\n")}
\`\`\`
`;

  return {
    title: topic.title,
    markdown: `${buildCommonHeader(topic.title, topic.path, siteOrigin)}

## Summary

${topic.summary}

## Sections on this page

${sectionHeadings}
${canonical == null ? "" : `
## Canonical markdown

- [${canonical.label}](${canonical.href})
`}
${quickActionBlock}
## Related pages

${related}
`,
    resources: canonical == null
      ? [
        { label: "Docs index", href: "/docs" },
      ]
      : [
        { label: "Public page", href: topic.path },
        canonical,
      ],
  };
}

type AppRouteDocument = {
  title: string;
  summary: string;
  bullets: ReadonlyArray<string>;
  agentTasks?: ReadonlyArray<string>;
  cautions?: ReadonlyArray<string>;
  contextLines?: ReadonlyArray<string>;
  resources?: ReadonlyArray<SiteAgentResource>;
  commands?: ReadonlyArray<string>;
};

const workspaceStudioTabDocs: Record<string, Omit<AppRouteDocument, "title" | "contextLines" | "resources"> & { titleSuffix: string }> = {
  browse: {
    titleSuffix: "Browse Files",
    summary: "Live file-tree view for the workspace head, optimized for inspecting directories, files, and working-copy state.",
    bullets: [
      "Use this tab to understand the current workspace tree and inspect files before making changes.",
      "It is the closest browser equivalent to looking at a mounted directory in your shell or editor.",
      "The format toggle changes how the tree is rendered, but the underlying workspace state is the same.",
    ],
    agentTasks: [
      "Confirm that the workspace contains the files the human expects before taking action.",
      "Mount the workspace locally if you need full shell or editor workflows against the same tree.",
      "Use this as the starting point before moving into checkpoints, changelog review, or settings.",
    ],
    commands: [
      "afs ws mount {{WORKSPACE}} ~/afs/{{WORKSPACE}}",
      "cd ~/afs/{{WORKSPACE}}",
      "find . -maxdepth 2 | head -40",
    ],
  },
  changes: {
    titleSuffix: "Changelog",
    summary: "Workspace-scoped change history for the current working copy, useful before checkpointing or reviewing recent edits.",
    bullets: [
      "Use this tab to see what changed in the workspace without leaving the browser.",
      "It focuses on working-copy mutations rather than long-range operational history.",
      "Open this before creating a checkpoint when you want to sanity-check the current delta.",
    ],
    agentTasks: [
      "Review the current delta before checkpointing or handing work to another agent.",
      "Correlate suspicious file changes with the broader activity stream if the source is unclear.",
      "Treat this as the browser-side readout of live workspace mutations, not as version control.",
    ],
    cautions: [
      "Changelog entries describe live workspace edits; they are not Git commits and do not replace explicit checkpoints.",
    ],
    commands: [
      "afs ws mount {{WORKSPACE}} ~/afs/{{WORKSPACE}}",
      "afs cp create {{WORKSPACE}} before-review",
      "afs cp list {{WORKSPACE}}",
    ],
  },
  checkpoints: {
    titleSuffix: "Checkpoints",
    summary: "Explicit savepoint management for the workspace, including restore-ready checkpoints around risky edits.",
    bullets: [
      "Use this tab to inspect prior checkpoints and create or restore safe rollback points.",
      "Checkpoints are explicit and durable; they are not created automatically when files change.",
      "This is the right surface before a broad refactor, agent handoff, or destructive experiment.",
    ],
    agentTasks: [
      "Create a checkpoint before risky edits or before delegating work to another agent.",
      "Use checkpoint history to understand safe restore targets when the workspace drifts.",
      "Pair this tab with Changelog when deciding whether the current working copy should be preserved.",
    ],
    commands: [
      "afs cp list {{WORKSPACE}}",
      "afs cp create {{WORKSPACE}} before-risky-change",
    ],
  },
  activity: {
    titleSuffix: "History",
    summary: "Workspace-specific event timeline for agent sessions, file actions, and checkpoint-related activity.",
    bullets: [
      "Use this tab when you need the timeline of what happened to this workspace, not just the current file tree.",
      "It complements the global History page by narrowing the view to one workspace.",
      "This is the best place to answer 'which agent touched this workspace, and when?'",
    ],
    agentTasks: [
      "Use the workspace timeline to correlate edits, session starts, and checkpoint events.",
      "Escalate to the global History page when you need to compare this workspace against cross-workspace activity.",
      "Jump back to Browse or Changelog once you know which files or moments need deeper inspection.",
    ],
  },
  settings: {
    titleSuffix: "Settings",
    summary: "Workspace metadata and destructive management actions, including rename, description edits, MCP access, and deletion.",
    bullets: [
      "Use this tab to manage workspace metadata rather than inspect files.",
      "This is where workspace-level MCP access and destructive actions are surfaced.",
      "The browser can jump from here into the MCP console with the workspace already scoped.",
    ],
    agentTasks: [
      "Verify that you are operating on the intended workspace before changing metadata or deleting anything.",
      "Use workspace-scoped MCP access when the agent should stay confined to one workspace.",
      "Treat this tab as administrative state, not content authoring state.",
    ],
    cautions: [
      "Deleting a workspace is irreversible from this surface.",
    ],
  },
};

const agentsTabDocs: Record<string, Omit<AppRouteDocument, "title" | "contextLines" | "resources"> & { titleSuffix: string }> = {
  active: {
    titleSuffix: "Active Agents",
    summary: "Live sessions connected to AFS right now, including which workspace each agent is attached to.",
    bullets: [
      "Use this tab for current connectivity and recency, not historical investigation.",
      "It is the fastest way to answer which agents are connected and where they are working.",
      "Open a workspace directly from here when you want to inspect the target of an active session.",
    ],
    agentTasks: [
      "Confirm whether the expected agent is alive before debugging a missing mutation or sync event.",
      "Use workspace filters to narrow the view when you only care about one workspace.",
      "Escalate to MCP or template setup if no sessions exist and one should be connected.",
    ],
  },
  history: {
    titleSuffix: "Connection History",
    summary: "Recent agent-session starts, useful for reconstructing when agents connected and to which workspace.",
    bullets: [
      "Use this tab for recent connection history rather than current heartbeat state.",
      "It is the browser audit trail for agent arrivals into the system.",
      "This complements the global History page with a session-focused slice.",
    ],
    agentTasks: [
      "Use this when someone asks when an agent last connected or whether a workspace ever received a session.",
      "Cross-check against workspace history if you need file effects after a session started.",
      "Clear filters when you suspect the relevant session is outside the current workspace or database scope.",
    ],
  },
};

const activityViewDocs: Record<string, Omit<AppRouteDocument, "title" | "contextLines" | "resources"> & { titleSuffix: string }> = {
  changes: {
    titleSuffix: "Changelog",
    summary: "Cross-workspace file-change feed, paginated for reviewing working-copy mutations across the product.",
    bullets: [
      "Use this view when the question is about changed files and byte deltas across all workspaces.",
      "It is broader than a single workspace changelog and narrower than general event history.",
      "Open a row to jump into the affected workspace with the relevant tab already selected.",
    ],
    agentTasks: [
      "Use this to identify which workspace changed before drilling into the specific workspace studio.",
      "Scan totals and deltas to spot unusually large or destructive edits.",
      "Treat this as an operational change feed, not a semantic code review.",
    ],
  },
  events: {
    titleSuffix: "History",
    summary: "Cross-workspace event timeline covering checkpoints, file actions, sessions, and broader system activity.",
    bullets: [
      "Use this when the question is 'what happened?' rather than 'which files changed?'",
      "This is the broadest audit surface in the signed-in product.",
      "Rows can route you back into the relevant workspace with a likely matching tab selected.",
    ],
    agentTasks: [
      "Use this view to reconstruct incidents or handoff timelines across multiple workspaces.",
      "Switch back to Changelog when you specifically need file-level delta details.",
      "Follow the linked workspace route once you know which target needs deeper investigation.",
    ],
  },
};

const appRouteDocuments: ReadonlyArray<{
  match: (pathname: string) => boolean;
  build: (context: SiteAgentRouteContext) => AppRouteDocument;
}> = [
  {
    match: (pathname) => pathname === "/",
    build: () => ({
      title: "Monitor",
      summary: "Operational overview of current workspaces, active agent sessions, and live activity.",
      bullets: [
        "Use this page to see what the CLI and connected agents are doing right now.",
        "The primary surfaces are the activity stream, active agents panel, quickstart cues, and template entry points.",
        "This is the signed-in overview, not the public marketing home page.",
      ],
      agentTasks: [
        "Start here when you need to answer 'what is happening now?' before drilling into a specific workspace.",
        "Use the live signals on this page to decide whether to jump into Workspaces, Agents, or Templates next.",
        "If no activity or agents are present, fall back to quickstart, CLI connection, or MCP setup flows.",
      ],
      resources: [
        { label: "Workspaces", href: "/workspaces" },
        { label: "Agents", href: "/agents" },
        { label: "History", href: "/activity" },
        { label: "Guide", href: `${docsBaseHref}/guides/agent-filesystem.md` },
      ],
      commands: [
        "afs status",
        "afs ws mount getting-started ~/getting-started",
      ],
    }),
  },
  {
    match: (pathname) => pathname === "/workspaces",
    build: () => ({
      title: "Workspaces",
      summary: "Workspace catalog and creation flow for the file trees your agents can access.",
      bullets: [
        "Use this page to browse, create, and open workspaces.",
        "This is the management view before drilling into one workspace.",
        "Each workspace is a live filesystem-shaped tree with checkpoints and activity.",
      ],
      agentTasks: [
        "Use this page to find the correct workspace before taking any file or checkpoint action.",
        "Create a new workspace here when a task should not reuse an existing working tree.",
        "Move into workspace studio once you know which workspace should be inspected or modified.",
      ],
      resources: [
        { label: "Monitor", href: "/" },
        { label: "Templates", href: "/templates" },
        { label: "Guide", href: `${docsBaseHref}/guides/agent-filesystem.md` },
      ],
      commands: [
        "afs ws create my-project",
        "afs ws mount my-project ~/my-project",
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/workspaces/"),
    build: ({ pathname, searchParams }) => {
      const workspaceId = decodeURIComponent(pathname.split("/")[2] ?? "workspace");
      const activeTab = searchParams.get("tab") ?? "browse";
      const activeDoc = workspaceStudioTabDocs[activeTab] ?? workspaceStudioTabDocs.browse;
      const databaseId = searchParams.get("databaseId");
      const showWelcome = searchParams.get("welcome") === "true";
      return {
        title: `Workspace Studio: ${activeDoc.titleSuffix}`,
        summary: `Detailed workspace view for \`${workspaceId}\`. ${activeDoc.summary}`,
        bullets: activeDoc.bullets,
        agentTasks: activeDoc.agentTasks,
        cautions: activeDoc.cautions,
        contextLines: [
          `Workspace id: \`${workspaceId}\``,
          `Active tab: ${activeDoc.titleSuffix}`,
          ...(databaseId ? [`Database scope: \`${databaseId}\``] : []),
          ...(showWelcome ? ["Welcome interstitial is active for a newly created workspace."] : []),
        ],
        resources: [
          { label: "All Workspaces", href: "/workspaces" },
          { label: "History", href: "/activity" },
          { label: "Guide", href: `${docsBaseHref}/guides/agent-filesystem.md` },
        ],
        commands: substituteWorkspaceCommands(activeDoc.commands, workspaceId),
      };
    },
  },
  {
    match: (pathname) => pathname.startsWith("/agents"),
    build: ({ searchParams }) => {
      const tab = searchParams.get("tab") ?? "active";
      const tabDoc = agentsTabDocs[tab] ?? agentsTabDocs.active;
      const workspaceId = searchParams.get("workspaceId");
      const databaseId = searchParams.get("databaseId");
      return {
        title: `Agents: ${tabDoc.titleSuffix}`,
        summary: tabDoc.summary,
        bullets: tabDoc.bullets,
        agentTasks: tabDoc.agentTasks,
        contextLines: [
          `Active tab: ${tabDoc.titleSuffix}`,
          ...(workspaceId ? [`Filtered workspace: \`${workspaceId}\``] : []),
          ...(databaseId ? [`Filtered database: \`${databaseId}\``] : []),
        ],
        resources: [
          { label: "Monitor", href: "/" },
          { label: "MCP", href: "/mcp" },
          { label: "Agent Guide", href: "/agent-guide" },
        ],
        commands: [
          "afs mcp",
        ],
      };
    },
  },
  {
    match: (pathname) => pathname === "/mcp/connect",
    build: ({ controlPlaneUrl }) => {
      const endpoint = `${controlPlaneUrl.replace(/\/+$/, "")}/mcp`;
      return {
        title: "MCP: Connect an Agent",
        summary: "Client-specific instructions for wiring an external agent runtime to the hosted AFS MCP endpoint.",
        bullets: [
          "This page is instruction-first: create a token, register the MCP server, then verify the client can talk to AFS.",
          "It covers both Codex and Claude Code against the same hosted endpoint.",
          "The browser UI contains interactive tabs, but the underlying endpoint and token model are shared.",
        ],
        agentTasks: [
          "Choose control-plane scope when the agent should create or manage workspaces itself.",
          "Choose workspace scope when the agent should only touch one workspace.",
          "Verify the client after setup so you know the token and endpoint are both correct.",
        ],
        cautions: [
          "Bearer tokens are shown once at creation time and should be treated like secrets.",
        ],
        contextLines: [
          `Hosted MCP endpoint: \`${endpoint}\``,
        ],
        resources: [
          { label: "MCP Console", href: "/mcp" },
          { label: "Agent Guide", href: "/agent-guide" },
          { label: "MCP reference", href: `${docsBaseHref}/reference/mcp.md` },
        ],
        commands: [
          "export AFS_TOKEN='<PASTE_TOKEN>'",
          `codex mcp add agent-filesystem --transport http '${endpoint}' --bearer-token-env AFS_TOKEN`,
          "",
          `claude mcp add --scope user --transport http agent-filesystem '${endpoint}' --header 'Authorization: Bearer <PASTE_TOKEN>'`,
        ],
      };
    },
  },
  {
    match: (pathname) => pathname.startsWith("/mcp"),
    build: ({ searchParams }) => ({
      title: "MCP",
      summary: "MCP connection and token-management surface for attaching external agents to AFS workspaces.",
      bullets: [
        "Use this page to create and manage agent access.",
        "This is where workspace-scoped and control-plane scoped credentials are surfaced, filtered, and revoked.",
        "Pair this page with the Agent Guide when handing AFS context to another agent runtime.",
      ],
      agentTasks: [
        "Create the narrowest token scope that still lets the agent complete its work.",
        "Use filters to audit only the tokens relevant to one workspace or database.",
        "Revoke stale tokens instead of reusing credentials with unclear provenance.",
      ],
      contextLines: [
        ...(searchParams.get("workspaceId") ? [`Filtered workspace: \`${searchParams.get("workspaceId")}\``] : []),
        ...(searchParams.get("databaseId") ? [`Filtered database: \`${searchParams.get("databaseId")}\``] : []),
      ],
      resources: [
        { label: "Connect an agent", href: "/mcp/connect" },
        { label: "Agent Guide", href: "/agent-guide" },
        { label: "MCP reference", href: `${docsBaseHref}/reference/mcp.md` },
      ],
      commands: [
        "afs mcp",
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/databases"),
    build: () => ({
      title: "Databases",
      summary: "Database inventory and management view for the Redis backends where workspaces live.",
      bullets: [
        "Use this page to add, inspect, and manage databases.",
        "Database availability determines where new workspaces can be created.",
        "This is infrastructure-facing state rather than workspace content.",
      ],
      agentTasks: [
        "Confirm database health and default selection before creating new workspaces.",
        "Use catalog repair when the control plane metadata looks stale or incomplete.",
        "Treat this page as infrastructure configuration, not day-to-day workspace editing.",
      ],
      resources: [
        { label: "Workspaces", href: "/workspaces" },
        { label: "Settings", href: "/settings" },
        { label: "Self-managed", href: `${docsBaseHref}/internals/cloud.md` },
      ],
      commands: [
        "afs database list",
        "afs database use my-database",
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/activity"),
    build: ({ searchParams }) => {
      const view = searchParams.get("view") ?? "changes";
      const viewDoc = activityViewDocs[view] ?? activityViewDocs.changes;
      return {
        title: `History: ${viewDoc.titleSuffix}`,
        summary: viewDoc.summary,
        bullets: viewDoc.bullets,
        agentTasks: viewDoc.agentTasks,
        contextLines: [
          `Active view: ${viewDoc.titleSuffix}`,
        ],
        resources: [
          { label: "Monitor", href: "/" },
          { label: "Workspaces", href: "/workspaces" },
        ],
      };
    },
  },
  {
    match: (pathname) => pathname.startsWith("/templates/installed/"),
    build: ({ pathname, searchParams }) => {
      const workspaceId = decodeURIComponent(pathname.split("/")[3] ?? "workspace");
      const databaseId = searchParams.get("databaseId");
      return {
        title: "Installed Template",
        summary: `Revisit a template-backed workspace and its setup instructions for \`${workspaceId}\`.`,
        bullets: [
          "Use this page to reopen template instructions after the workspace has already been created.",
          "It can repair missing template MCP tokens, deep-link back into the workspace, or uninstall the template workspace.",
          "This page is about the installed template instance, not the gallery of available templates.",
        ],
        agentTasks: [
          "Open the workspace from here when you need to inspect or modify the installed files directly.",
          "Repair the template token when the setup instructions need a fresh bearer credential.",
          "Use uninstall only when you intentionally want to revoke template access and delete the workspace.",
        ],
        cautions: [
          "Uninstall revokes active template tokens and deletes the workspace.",
        ],
        contextLines: [
          `Installed workspace id: \`${workspaceId}\``,
          ...(databaseId ? [`Database scope: \`${databaseId}\``] : []),
        ],
        resources: [
          { label: "Templates", href: "/templates" },
          { label: "Workspace", href: `/workspaces/${workspaceId}${databaseId ? `?databaseId=${encodeURIComponent(databaseId)}` : ""}` },
          { label: "MCP", href: `/mcp${databaseId ? `?databaseId=${encodeURIComponent(databaseId)}&workspaceId=${encodeURIComponent(workspaceId)}` : `?workspaceId=${encodeURIComponent(workspaceId)}`}` },
        ],
        commands: [
          `afs ws mount ${workspaceId} ~/afs/${workspaceId}`,
          `afs cp list ${workspaceId}`,
        ],
      };
    },
  },
  {
    match: (pathname) => pathname.startsWith("/templates"),
    build: () => ({
      title: "Workspace Templates",
      summary: "Seeded workspace starting points for common multi-agent workflows.",
      bullets: [
        "Use this page when you want a prepared workspace instead of starting blank.",
        "The page has two modes of value: installed templates you can revisit, and a gallery of templates you can start from.",
        "Templates include prompt framing, install details, and initial file structure for repeatable agent workflows.",
      ],
      agentTasks: [
        "Prefer a template when the task matches an existing workflow pattern instead of reinventing setup from scratch.",
        "Open an installed template when you need to recover its instructions or token state after the initial setup.",
        "Move into Workspaces after install if the task is now about file edits rather than bootstrapping.",
      ],
      resources: [
        { label: "Workspaces", href: "/workspaces" },
        { label: "Agent Guide", href: "/agent-guide" },
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/settings"),
    build: () => ({
      title: "Settings",
      summary: "Account and environment settings, including developer reset and account-level management actions.",
      bullets: [
        "Use this page for account maintenance rather than workspace work.",
        "Some actions here are destructive, such as resets or deletion flows.",
        "This is where you verify or change your account-level AFS state.",
      ],
      agentTasks: [
        "Use this page for account-level cleanup, skin changes, or onboarding reset rather than workspace content.",
        "Verify auth mode before assuming all account actions are available in the UI.",
        "Pause before destructive account actions because they affect more than one workspace.",
      ],
      cautions: [
        "Developer reset and full account deletion can remove owned databases and workspaces.",
      ],
      resources: [
        { label: "Databases", href: "/databases" },
        { label: "Monitor", href: "/" },
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/admin"),
    build: () => ({
      title: "Admin",
      summary: "Cloud operator surface for cross-tenant users, databases, workspaces, and agents.",
      bullets: [
        "Use this page only when you have admin visibility enabled.",
        "This is an operator view across the whole service, not just your own workspaces.",
        "It is meant for supervision and intervention, not daily workspace editing.",
      ],
      agentTasks: [
        "Use this page for operator-level inspection when a tenant or database issue spans multiple users.",
        "Avoid treating admin visibility as a substitute for workspace-scoped investigation when the issue is isolated.",
        "Be explicit about tenant boundaries and blast radius before taking action here.",
      ],
      cautions: [
        "Admin actions can affect data outside your personal workspace scope.",
      ],
      resources: [
        { label: "Databases", href: "/databases" },
        { label: "History", href: "/activity" },
      ],
    }),
  },
  {
    match: (pathname) => pathname.startsWith("/connect-cli"),
    build: ({ searchParams }) => {
      const connected = searchParams.get("connected") === "true";
      const workspaceName = searchParams.get("workspace_name");
      const canonicalName = workspaceName == null ? null : canonicalWorkspaceName(workspaceName);
      const mountCommand = canonicalName == null
        ? "afs ws mount"
        : `afs ws mount ${canonicalName} ~/afs/${canonicalName}`;

      return {
        title: connected ? "Connect CLI: Success" : "Connect CLI",
        summary: connected
          ? "Browser handoff is complete and the terminal can now mount the selected workspace."
          : "Browser handoff flow that links a local CLI session to an authenticated AFS account and workspace.",
        bullets: connected
          ? [
            "The browser-side auth step is complete; the next action is back in the terminal.",
            "If a workspace was selected, the page surfaces the exact mount command to run next.",
            "This page is transient and exists only to bridge CLI login back through the browser.",
          ]
          : [
            "This page exists to bridge browser auth back into a terminal session.",
            "It can auto-connect the terminal to the getting-started workspace or a selected workspace.",
            "After the handoff, return to the shell to run the mount command it gives you.",
          ],
        agentTasks: connected
          ? [
            "Return to the shell and run the mount command shown here.",
            "Open Workspaces afterward if you want to choose a different workspace or create another one.",
          ]
          : [
            "Confirm the browser is pairing the CLI with the intended workspace before completing the redirect.",
            "If no workspaces exist, create one first or use the getting-started path when available.",
            "Treat return URL failures as CLI handoff issues, not as workspace content issues.",
          ],
        contextLines: [
          ...(workspaceName ? [`Workspace: ${displayWorkspaceName(workspaceName)}`] : []),
          ...(searchParams.get("workspace") ? [`Workspace hint: \`${searchParams.get("workspace")}\``] : []),
          ...(searchParams.get("state") ? ["CLI handoff state token is present."] : []),
        ],
        resources: [
          { label: "Downloads", href: "/downloads" },
          { label: "Workspaces", href: "/workspaces" },
          { label: "CLI reference", href: `${docsBaseHref}/reference/cli.md` },
        ],
        commands: [
          mountCommand,
        ],
      };
    },
  },
  {
    match: (pathname) => pathname.startsWith("/login") || pathname.startsWith("/signup") || pathname.startsWith("/forgot-password") || pathname.startsWith("/sso-callback"),
    build: ({ pathname }) => ({
      title: authRouteTitle(pathname),
      summary: "Authentication flow for entering or recovering access to the AFS web app.",
      bullets: [
        "Use this page to sign in, create an account, or finish an auth redirect.",
        "After auth succeeds, the app routes unlock and the same Human/Agent switcher remains available there.",
      ],
      agentTasks: [
        "Treat this as access setup, not a workspace operation surface.",
        "Return to the original target route after auth if the user was deep-linking into the app.",
      ],
      resources: [
        { label: "Home", href: "/home" },
        { label: "Docs", href: "/docs" },
      ],
    }),
  },
];

function buildAppDocument(context: SiteAgentRouteContext): SiteAgentDocument | null {
  const matched = appRouteDocuments.find((item) => item.match(context.pathname));
  if (matched == null) {
    return null;
  }

  const document = matched.build(context);
  const navTitle = resolveNavigationTitleParts(context.pathname);
  const commandBlock = document.commands == null ? "" : `
## Quick actions

\`\`\`bash
${document.commands.join("\n")}
\`\`\`
`;
  const relatedLinks = document.resources?.map((resource) =>
    `- [${resource.label}](${resource.href.startsWith("/") ? joinUrl(context.siteOrigin, resource.href) : resource.href})`).join("\n") ?? buildAppSiteMap(context.siteOrigin);
  const contextBlock = [
    `- Page title: ${navTitle.page || document.title}`,
    ...(navTitle.section ? [`- Section: ${navTitle.section}`] : []),
    ...(navTitle.subtitle ? [`- Subtitle: ${navTitle.subtitle}`] : []),
    ...(document.contextLines ?? []).map((line) => `- ${line}`),
  ].join("\n");
  const agentTaskBlock = document.agentTasks == null ? "" : `
## What an agent should do here

${document.agentTasks.map((task) => `- ${task}`).join("\n")}
`;
  const cautionsBlock = document.cautions == null ? "" : `
## Cautions

${document.cautions.map((item) => `- ${item}`).join("\n")}
`;

  return {
    title: document.title,
    markdown: `${buildCommonHeader(document.title, context.pathname, context.siteOrigin)}

## Summary

${document.summary}

## Current route context

${contextBlock}

## What this page is for

${document.bullets.map((bullet) => `- ${bullet}`).join("\n")}
${agentTaskBlock}${cautionsBlock}${commandBlock}
## Related pages

${relatedLinks}
`,
    resources: document.resources,
  };
}

function substituteWorkspaceCommands(commands: ReadonlyArray<string> | undefined, workspaceId: string) {
  if (commands == null) {
    return undefined;
  }
  return commands.map((command) => command.replaceAll("{{WORKSPACE}}", workspaceId));
}

function buildCommonHeader(title: string, pathname: string, siteOrigin: string) {
  return `# ${title}

> Agent-first markdown view for [${pathname}](${joinUrl(siteOrigin, pathname)})

## Site map

${buildSiteMap(siteOrigin)}`;
}

function buildSiteMap(siteOrigin: string) {
  const links = publicNavItems
    .map((item) => `- [${item.label}](${joinUrl(siteOrigin, item.path)})`)
    .join("\n");
  return `- [Home](${joinUrl(siteOrigin, "/home")})
${links}
- [Monitor](${joinUrl(siteOrigin, "/")})
- [Workspaces](${joinUrl(siteOrigin, "/workspaces")})
- [Agents](${joinUrl(siteOrigin, "/agents")})
- [MCP](${joinUrl(siteOrigin, "/mcp")})
- [Databases](${joinUrl(siteOrigin, "/databases")})
- [History](${joinUrl(siteOrigin, "/activity")})
- [Templates](${joinUrl(siteOrigin, "/templates")})
- [Settings](${joinUrl(siteOrigin, "/settings")})
- [${publicRepoLink.label}](${publicRepoLink.href})`;
}

function buildAppSiteMap(siteOrigin: string) {
  const appLinks = [...navigationItems, ...bottomNavigationItems]
    .map((item) => item.kind === "route"
      ? `- [${item.label}](${joinUrl(siteOrigin, item.path)})`
      : item.children.map((child) => `- [${child.label}](${joinUrl(siteOrigin, child.path)})`).join("\n"))
    .join("\n");
  return `${appLinks}
- [Settings](${joinUrl(siteOrigin, "/settings")})`;
}

function buildBaseResources(): SiteAgentResource[] {
  return [
    { label: "Home", href: "/home" },
    { label: "Docs", href: "/docs" },
    { label: "Agent Guide", href: "/agent-guide" },
    { label: publicRepoLink.label, href: publicRepoLink.href },
  ];
}

function authRouteTitle(pathname: string) {
  if (pathname.startsWith("/signup")) return "Sign Up";
  if (pathname.startsWith("/forgot-password")) return "Forgot Password";
  if (pathname.startsWith("/sso-callback")) return "SSO Callback";
  return "Log In";
}

function normalizePublicPath(pathname: string) {
  if (pathname === "/") return pathname;
  return pathname.replace(/\/+$/, "");
}

function joinUrl(base: string, path: string) {
  const normalizedBase = base.replace(/\/+$/, "");
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;
  return `${normalizedBase}${normalizedPath}`;
}
