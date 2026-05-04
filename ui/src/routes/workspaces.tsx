// /workspaces — observability-flavored list view.
//
// Primary message: workspaces are created from the CLI (`afs ws create <name>`).
// The web UI lists what exists and lets you click into it, but doesn't expose
// inline create/edit/delete affordances. The corresponding mutations live in:
//
//   - first-run: the auto-provisioned getting-started workspace at `/`
//     (see routes/index.tsx). that flow stays for users with no CLI yet.
//   - CLI/MCP: `afs ws create`, `afs ws import`, `afs ws delete`, `afs ws fork`.
//   - workspace detail page: a "manual override" disclosure for delete in
//     case someone genuinely needs to act from the browser.
//
// This is intentional: an inline "Add workspace" button signals "this is a
// managed-service console." Removing it signals "the CLI is the actor; this
// page is the viewport."

import { createFileRoute, Outlet, useLocation, useNavigate, useRouter } from "@tanstack/react-router";
import { Loader } from "@redis-ui/components";
import { useState } from "react";
import styled from "styled-components";
import { PageStack } from "../components/afs-kit";
import {
  agentsQueryOptions,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";
import {
  useScopedAgents,
  useScopedWorkspaceSummaries,
} from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import type { AFSWorkspaceSummary } from "../foundation/types/afs";

const FREE_TIER_WORKSPACE_LIMIT = 3;

export const Route = createFileRoute("/workspaces")({
  loader: async () => {
    await Promise.all([
      queryClient.ensureQueryData({
        ...workspaceSummariesQueryOptions(null),
        revalidateIfStale: true,
      }),
      queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
    ]);
  },
  component: WorkspacesPage,
});

function workspaceRowKey(databaseId: string | undefined, workspaceId: string) {
  return `${databaseId ?? ""}:${workspaceId}`;
}

function WorkspacesPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const router = useRouter();
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();

  if (location.pathname !== "/workspaces") {
    return <Outlet />;
  }

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data;
  const connectedAgentsByWorkspace = agentsQuery.data.reduce<Record<string, number>>(
    (counts, session) => {
      const key = workspaceRowKey(session.databaseId, session.workspaceId);
      counts[key] = (counts[key] ?? 0) + 1;
      return counts;
    },
    {},
  );

  function openWorkspace(workspace: AFSWorkspaceSummary) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: { databaseId: workspace.databaseId },
    });
  }

  function previewWorkspace(workspace: AFSWorkspaceSummary) {
    void router.preloadRoute({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: { databaseId: workspace.databaseId },
    });
  }

  function openWorkspaceTab(
    workspace: AFSWorkspaceSummary,
    tab: "browse" | "checkpoints" | "activity" | "settings",
  ) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId: workspace.id },
      search: {
        databaseId: workspace.databaseId,
        ...(tab === "browse" ? {} : { tab }),
      },
    });
  }

  return (
    <PageStack>
      <CLICreatePanel workspaces={workspaces} />

      <WorkspaceTable
        rows={workspaces}
        loading={workspacesQuery.isLoading}
        error={workspacesQuery.isError}
        connectedAgentsByWorkspace={connectedAgentsByWorkspace}
        onOpenWorkspace={openWorkspace}
        onPreviewWorkspace={previewWorkspace}
        onOpenWorkspaceTab={openWorkspaceTab}
        // intentionally no onEditWorkspace / onDeleteWorkspace — managed via CLI.
      />
    </PageStack>
  );
}

// ──────────────────────────────────────────────────────────────────────
// CLICreatePanel — the "create from CLI" hint that replaces the old
// "Add workspace" toolbar button. Single line, copyable command, no modal.
//
// On free-tier accounts we surface the quota inline ("1 / 3 free") so the
// user knows the limit without needing to attempt a create and get blocked.
// ──────────────────────────────────────────────────────────────────────

function CLICreatePanel({ workspaces }: { workspaces: AFSWorkspaceSummary[] }) {
  const command = "afs ws create my-workspace";
  const [copied, setCopied] = useState(false);

  // Free-tier quota chip is purely informational here (no UI path can mutate).
  const onboardingDb = workspaces.find((ws) => ws.databaseId)?.databaseId; // any ws
  // count workspaces against the onboarding tier — same heuristic as before.
  const freeTierUsed = workspaces.filter((ws) => isOnboardingDatabase(ws.databaseName)).length;
  const showFreeTier = onboardingDb != null && freeTierUsed > 0;
  const freeTierExhausted = showFreeTier && freeTierUsed >= FREE_TIER_WORKSPACE_LIMIT;

  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard blocked; user can select manually */
    }
  }

  return (
    <PanelRoot>
      <PanelHead>
        <PanelEyebrow>create a workspace</PanelEyebrow>
        <PanelBody>
          Workspaces in AFS are created from the CLI. The web UI is for
          watching what your CLI and agents are doing, not provisioning.
        </PanelBody>
      </PanelHead>
      <PanelCommandRow>
        <PanelCommandPrompt>$</PanelCommandPrompt>
        <PanelCommand>{command}</PanelCommand>
        <CopyButton type="button" onClick={copyCommand} aria-label="Copy command">
          {copied ? "copied" : "copy"}
        </CopyButton>
      </PanelCommandRow>
      <PanelFootnote>
        New to the CLI? <PanelLink href="/docs/cli">install instructions →</PanelLink>{showFreeTier ? (
          <FreeTierInline $exhausted={freeTierExhausted}>
            {" · "}{freeTierUsed} / {FREE_TIER_WORKSPACE_LIMIT} free workspaces used
          </FreeTierInline>
        ) : null}
      </PanelFootnote>
    </PanelRoot>
  );
}

function isOnboardingDatabase(databaseName: string) {
  // best-effort: the onboarding tier is named "onboarding" in catalogs.
  return databaseName.toLowerCase().includes("onboarding");
}

// ──────────────────────────────────────────────────────────────────────
// styles
// ──────────────────────────────────────────────────────────────────────

const PanelRoot = styled.section`
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 18px 22px;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  background: var(--afs-panel);
`;

const PanelHead = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const PanelEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
`;

const PanelBody = styled.p`
  margin: 0;
  max-width: 64ch;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
`;

const PanelCommandRow = styled.div`
  display: flex;
  align-items: center;
  gap: 0;
  padding: 10px 14px;
  background: var(--afs-bg-soft);
  border: 1px solid var(--afs-line);
  border-radius: 8px;
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 13px;
`;

const PanelCommandPrompt = styled.span`
  color: var(--afs-muted);
  margin-right: 1ch;
  user-select: none;
`;

const PanelCommand = styled.code`
  flex: 1;
  color: var(--afs-ink);
  white-space: pre;
  overflow-x: auto;
`;

const CopyButton = styled.button`
  flex: 0 0 auto;
  font-family: var(--afs-mono, "Monaco", "Menlo", monospace);
  font-size: 11px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  background: transparent;
  color: var(--afs-accent);
  border: 1px solid var(--afs-accent);
  border-radius: 4px;
  padding: 3px 9px;
  cursor: pointer;

  &:hover {
    background: var(--afs-accent);
    color: var(--afs-ink-on-accent);
  }
`;

const PanelFootnote = styled.div`
  font-size: 12px;
  color: var(--afs-muted);
`;

const PanelLink = styled.a`
  color: var(--afs-accent);
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;

const FreeTierInline = styled.span<{ $exhausted?: boolean }>`
  color: ${(p) => (p.$exhausted ? "#b91c1c" : "var(--afs-muted)")};
`;
