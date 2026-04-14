import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { useEffect, useRef, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  EmptyState,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  SectionCard,
  SectionGrid,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { ConnectAgentBanner } from "../components/connect-agent-banner";
import { AgentConnectedDialog } from "../components/agent-connected-dialog";
import {
  useDeleteWorkspaceMutation,
  useWorkspace,
} from "../foundation/hooks/use-afs";
import { useDatabaseScope } from "../foundation/database-scope";
import type { AFSAgentSession, AFSWorkspaceView } from "../foundation/types/afs";
import { OverviewTab } from "./workspace-studio/-overview-tab";
import { FilesTab } from "./workspace-studio/-files-tab";
import { CheckpointsTab } from "./workspace-studio/-checkpoints-tab";
import { ActivityTab } from "./workspace-studio/-activity-tab";

type StudioTab = "overview" | "files" | "checkpoints" | "activity";

const workspaceStudioSearchSchema = z.object({
  tab: z.enum(["overview", "files", "checkpoints", "activity"]).optional(),
  welcome: z.boolean().optional(),
});

export const Route = createFileRoute("/workspaces/$workspaceId")({
  validateSearch: workspaceStudioSearchSchema,
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
  const hadAgentsBefore = useRef(false);

  const workspace = workspaceQuery.data;
  const tab = search.tab ?? "overview";
  const hasAgents = (workspace?.agents?.length ?? 0) > 0;
  // Show the banner when: no agents yet, or the agent-connected dialog is up,
  // or the user just dismissed the dialog and we're showing "What's Next".
  const showBanner = workspace != null && !bannerDismissed &&
    (!hasAgents || connectedAgent != null || showWhatsNext);

  // Always poll while on this page so we detect agent connections promptly.
  useEffect(() => {
    const interval = setInterval(() => {
      void workspaceQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Detect first agent connection: hasAgents transitions false → true.
  useEffect(() => {
    if (hasAgents && !hadAgentsBefore.current) {
      const agent = workspace?.agents?.[0] ?? null;
      if (agent) {
        setConnectedAgent(agent);
      }
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
      search: nextTab === "overview"
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
      `Delete workspace "${workspace?.name ?? workspaceId}"? This removes it from the workspace registry.`,
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
          workspaceName={workspace.name}
          onDismiss={() => {
            setBannerDismissed(true);
            setShowWhatsNext(false);
            // Remove the welcome param from URL if present.
            if (search.welcome) {
              void navigate({
                to: "/workspaces/$workspaceId",
                params: { workspaceId },
                search: tab === "overview" ? {} : { tab },
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
            Workspaces
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
        <TabButton $active={tab === "overview"} onClick={() => setStudioTab("overview")}>
          Overview
        </TabButton>
        <TabButton $active={tab === "files"} onClick={() => setStudioTab("files")}>
          Files
        </TabButton>
        <TabButton $active={tab === "checkpoints"} onClick={() => setStudioTab("checkpoints")}>
          Checkpoints
        </TabButton>
        <TabButton $active={tab === "activity"} onClick={() => setStudioTab("activity")}>
          Activity
        </TabButton>
      </Tabs>

      {tab === "overview" ? <OverviewTab workspace={workspace} /> : null}

      {tab === "files" ? (
        <FilesTab
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
          onTabChange={setStudioTab}
        />
      ) : null}

      {/* Danger zone */}
      <DangerZoneCard>
        <DangerZoneHeader>
          <DangerZoneTitle>Danger zone</DangerZoneTitle>
          <DangerZoneDesc>
            Permanently delete this workspace and remove it from the registry.
          </DangerZoneDesc>
        </DangerZoneHeader>
        <DeleteWorkspaceButton
          size="large"
          disabled={deleteWorkspace.isPending}
          onClick={deleteCurrentWorkspace}
        >
          {deleteWorkspace.isPending ? "Deleting..." : "Delete workspace"}
        </DeleteWorkspaceButton>
      </DangerZoneCard>

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
  border: none;
  background: transparent;
  padding: 0;
  color: var(--afs-ink);
  font: inherit;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;

  &:hover {
    text-decoration: underline;
  }
`;

const BreadcrumbSeparator = styled.span`
  color: var(--afs-muted);
  font-size: 14px;
`;

const BreadcrumbCurrent = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 400;
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

const DangerZoneCard = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-top: 24px;
  padding: 20px 24px;
  border: 1px solid rgba(220, 38, 38, 0.2);
  border-radius: 16px;
  background: rgba(220, 38, 38, 0.03);

  @media (max-width: 720px) {
    flex-direction: column;
    align-items: flex-start;
  }
`;

const DangerZoneHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 4px;
`;

const DangerZoneTitle = styled.h3`
  margin: 0;
  color: #dc2626;
  font-size: 15px;
  font-weight: 700;
`;

const DangerZoneDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.5;
`;

const DeleteWorkspaceButton = styled(Button)`
  && {
    white-space: nowrap;
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }

  &&:hover:not(:disabled),
  &&:focus-visible:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
    box-shadow: none;
  }
`;
