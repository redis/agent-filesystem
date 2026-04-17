import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useRef, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  EmptyState,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { ConnectAgentBanner } from "../components/connect-agent-banner";
import { AgentConnectedDialog } from "../components/agent-connected-dialog";
import {
  useDeleteWorkspaceMutation,
  useWorkspace,
  workspaceQueryOptions,
} from "../foundation/hooks/use-afs";
import { useDatabaseScope } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { studioTabSchema } from "../foundation/workspace-tabs";
import type { StudioTab } from "../foundation/workspace-tabs";
import type { AFSAgentSession, AFSWorkspaceView } from "../foundation/types/afs";
import { BrowseTab } from "./workspace-studio/-browse-tab";
import { CheckpointsTab } from "./workspace-studio/-checkpoints-tab";
import { ActivityTab } from "./workspace-studio/-activity-tab";
import { SettingsTab } from "./workspace-studio/-settings-tab";

const workspaceStudioSearchSchema = z.object({
  tab: studioTabSchema.optional(),
  welcome: z.boolean().optional(),
});

export const Route = createFileRoute("/workspaces/$workspaceId")({
  validateSearch: workspaceStudioSearchSchema,
  loader: ({ params }) =>
    queryClient.ensureQueryData({
      ...workspaceQueryOptions(null, params.workspaceId),
      revalidateIfStale: true,
    }),
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const search = Route.useSearch();
  const { unavailableDatabases } = useDatabaseScope();
  const workspaceQuery = useWorkspace(null, workspaceId);
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [browserView, setBrowserView] = useState<AFSWorkspaceView>("head");
  const [bannerDismissed, setBannerDismissed] = useState(false);
  const [connectedAgent, setConnectedAgent] = useState<AFSAgentSession | null>(null);
  // Keeps the banner pinned to "What's Next" after the dialog is dismissed.
  const [showWhatsNext, setShowWhatsNext] = useState(false);
  const bannerStepRef = useRef<{ jumpToStep: (s: 1 | 2 | 3) => void } | null>(null);
  // Initialize to current agent state so we only detect *new* connections,
  // not agents that were already there when the page loaded.
  const hadAgentsBefore = useRef<boolean | null>(null);

  const workspace = workspaceQuery.data;
  const tab = search.tab ?? "browse";
  const hasAgents = (workspace?.agents.length ?? 0) > 0;
  // Show the banner only during the first-time setup flow (welcome=true in URL),
  // or when the agent-connected dialog / "What's Next" step is active.
  const showBanner = workspace != null && !bannerDismissed &&
    (search.welcome || connectedAgent != null || showWhatsNext);

  // Always poll while on this page so we detect agent connections promptly.
  useEffect(() => {
    const interval = setInterval(() => {
      void workspaceQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, []);

  // Detect first agent connection: hasAgents transitions false → true.
  // Wait until workspace data is loaded before seeding, so we don't
  // mistake the loading→loaded transition for a new agent connecting.
  useEffect(() => {
    if (workspace == null) return; // Still loading — don't seed yet.
    if (hadAgentsBefore.current === null) {
      // First time we have real data — seed with current state, don't trigger.
      hadAgentsBefore.current = hasAgents;
      return;
    }
    if (hasAgents && !hadAgentsBefore.current) {
      setConnectedAgent(workspace.agents[0]);
    }
    hadAgentsBefore.current = hasAgents;
  }, [hasAgents, workspace]);

  useEffect(() => {
    if (workspace == null) {
      setBrowserView("head");
      return;
    }

    const defaultView = workspace.capabilities.browseWorkingCopy ? "working-copy" : "head";
    setBrowserView(defaultView);
  }, [workspace]);

  function setStudioTab(nextTab: StudioTab) {
    void navigate({
      to: "/workspaces/$workspaceId",
      params: { workspaceId },
      search: nextTab === "browse"
        ? {}
        : { tab: nextTab },
      replace: true,
    });
  }

  function dismissConnectedDialog() {
    setConnectedAgent(null);
    // If the getting-started banner was showing, keep it visible on step 3.
    if (!bannerDismissed) {
      setShowWhatsNext(true);
      // Small delay so the ref is mounted before we call jumpToStep.
      setTimeout(() => bannerStepRef.current?.jumpToStep(3), 0);
    }
  }

  function deleteCurrentWorkspace() {
    if (workspace == null) {
      return;
    }
    const confirmed = window.confirm(
      `Delete workspace "${workspace.name}"? This removes it from the workspace registry.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate({
      workspaceId,
    }, {
      onSuccess: () => {
        void navigate({ to: "/workspaces" });
      },
    });
  }

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (workspaceQuery.isError) {
    return (
      <PageStack>
        <EmptyState role="alert">
          <NoticeTitle>Workspace unavailable</NoticeTitle>
          <NoticeBody>
            {workspaceQuery.error instanceof Error
              ? workspaceQuery.error.message
              : "This workspace could not be loaded right now."}
          </NoticeBody>
          {unavailableDatabases.length > 0 ? (
            <NoticeBody>
              Disconnected databases:{" "}
              {unavailableDatabases.map((database) => database.displayName || database.databaseName).join(", ")}.
            </NoticeBody>
          ) : null}
        </EmptyState>
      </PageStack>
    );
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  return (
    <PageStack>
      {unavailableDatabases.length > 0 ? (
        <NoticeCard $tone="warning" role="status">
          <NoticeTitle>Some databases are unavailable</NoticeTitle>
          <NoticeBody>
            Workspace browsing will continue for healthy backends, but data from disconnected databases may be incomplete.
          </NoticeBody>
        </NoticeCard>
      ) : null}

      {showBanner ? (
        <ConnectAgentBanner
          ref={bannerStepRef}
          workspaceId={workspaceId}
          workspaceName={workspace.name}
          onDismiss={() => {
            setBannerDismissed(true);
            setShowWhatsNext(false);
            // Remove the welcome param from URL if present.
            if (search.welcome) {
              void navigate({
                to: "/workspaces/$workspaceId",
                params: { workspaceId },
                search: tab === "browse" ? {} : { tab },
                replace: true,
              });
            }
          }}
        />
      ) : null}

      <StudioNavRow>
        <BreadcrumbGroup>
          <BreadcrumbButton
            type="button"
            onClick={() => {
              void navigate({ to: "/workspaces" });
            }}
          >
            <BackArrow aria-hidden>&#8592;</BackArrow>
            Back to Workspaces
          </BreadcrumbButton>
          <BreadcrumbSeparator>/</BreadcrumbSeparator>
          <BreadcrumbCurrent>{workspace.name}</BreadcrumbCurrent>
        </BreadcrumbGroup>
        <StudioActions>
          {hasAgents ? (
            <ViewAgentsButton
              kind="ghost"
              size="large"
              onClick={() => {
                void navigate({
                  to: "/agents",
                  search: {
                    workspaceId,
                  },
                });
              }}
            >
              View agents
            </ViewAgentsButton>
          ) : (
            <ConnectAgentButton
              kind="ghost"
              size="large"
              onClick={() => setBannerDismissed(false)}
            >
              Connect agent
            </ConnectAgentButton>
          )}
        </StudioActions>
      </StudioNavRow>

      <Tabs>
        <TabButton $active={tab === "browse"} onClick={() => setStudioTab("browse")}>
          Browse
        </TabButton>
        <TabButton $active={tab === "checkpoints"} onClick={() => setStudioTab("checkpoints")}>
          Checkpoints
        </TabButton>
        <TabButton $active={tab === "activity"} onClick={() => setStudioTab("activity")}>
          Activity
        </TabButton>
        <TabButton $active={tab === "settings"} onClick={() => setStudioTab("settings")}>
          Settings
        </TabButton>
      </Tabs>

      {tab === "browse" ? (
        <BrowseTab
          workspace={workspace}
          browserView={browserView}
          onBrowserViewChange={setBrowserView}
        />
      ) : null}

      {tab === "checkpoints" ? (
        <CheckpointsTab
          workspace={workspace}
          onBrowserViewChange={setBrowserView}
          onTabChange={setStudioTab}
        />
      ) : null}

      {tab === "activity" ? (
        <ActivityTab
          activity={workspace.activity}
          updatedAt={workspace.updatedAt}
          onTabChange={setStudioTab}
        />
      ) : null}

      {tab === "settings" ? (
        <SettingsTab
          workspace={workspace}
          onDelete={deleteCurrentWorkspace}
          isDeleting={deleteWorkspace.isPending}
        />
      ) : null}

      {/* Agent connected pop-up dialog */}
      {connectedAgent ? (
        <AgentConnectedDialog
          agent={connectedAgent}
          onClose={dismissConnectedDialog}
        />
      ) : null}
    </PageStack>
  );
}

const StudioNavRow = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  min-height: 24px;
`;

const StudioActions = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;

  @media (max-width: 720px) {
    width: 100%;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
`;

const BreadcrumbGroup = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
`;

const BreadcrumbButton = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: none;
  background: transparent;
  padding: 0;
  color: var(--afs-muted);
  font: inherit;
  font-size: 14px;
  font-weight: 400;
  cursor: pointer;

  &:hover {
    color: var(--afs-ink);
    text-decoration: underline;
  }
`;

const BackArrow = styled.span`
  font-size: 16px;
  line-height: 1;
`;

const BreadcrumbSeparator = styled.span`
  color: var(--afs-muted);
  font-size: 14px;
`;

const BreadcrumbCurrent = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const ViewAgentsButton = styled(Button)`
  && {
    white-space: nowrap;
    box-shadow: none;
  }
`;

const ConnectAgentButton = styled(Button)`
  && {
    white-space: nowrap;
    box-shadow: none;
  }
`;
