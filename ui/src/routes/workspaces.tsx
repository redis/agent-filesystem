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
import { useMemo } from "react";
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
import { useDrawer, useDrawerCommands } from "../foundation/drawer-context";
import type { CommandsDrawerConfig } from "../foundation/drawer-context";

const FREE_TIER_WORKSPACE_LIMIT = 3;

const WORKSPACE_COMMANDS: CommandsDrawerConfig = {
  title: "Work with workspaces",
  subline: "Common commands. Run from your shell.",
  sections: [
    {
      title: "Create",
      description: "Provision a new workspace.",
      command: "afs ws create my-workspace",
    },
    {
      title: "Mount",
      description: "Mount the workspace into a local path.",
      command: "afs ws mount my-workspace ~/afs/my-workspace",
    },
    {
      title: "Fork",
      description: "Branch a copy for an experiment.",
      command: "afs ws fork my-workspace my-experiment",
    },
    {
      title: "Delete",
      description: "Tear down a workspace you no longer need.",
      command: "afs ws delete my-workspace",
    },
  ],
};

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
  // Register contextual commands so the global Help button opens this page's
  // workspace command reference. Memoized so the registration is stable.
  const drawerConfig = useMemo(() => WORKSPACE_COMMANDS, []);
  useDrawerCommands(
    location.pathname === "/workspaces" ? drawerConfig : null,
  );

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
    tab: "browse" | "checkpoints" | "changes" | "settings",
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

