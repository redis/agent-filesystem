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
  const showWelcomeInterstitial = workspace != null && search.welcome && connectedAgent == null && !showWhatsNext;

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

  const workspaceLabel = workspace.name === "getting-started" ? "Getting-started" : workspace.name;

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

      {showWelcomeInterstitial ? (
        <WelcomeInterstitial>
          <WelcomeCard>
            <WelcomeHeader>
              <WelcomeIconWrap aria-hidden>
                <WelcomeIconSpark>✦</WelcomeIconSpark>
              </WelcomeIconWrap>
              <WelcomeHeaderCopy>
                <WelcomeEyebrow>Step 1 of 2 • AFS Cloud</WelcomeEyebrow>
                <WelcomeTitle>{workspaceLabel} is ready</WelcomeTitle>
              </WelcomeHeaderCopy>
            </WelcomeHeader>
            <WelcomeBody>
              Congrats. We created your first workspace, <WelcomeWorkspaceName>{workspaceLabel}</WelcomeWorkspaceName>, inside the shared Getting Started database and loaded it with sample files so you can explore AFS immediately.
            </WelcomeBody>
            <WelcomeFacts>
              <WelcomeFact>
                <WelcomeFactValue>{workspace.fileCount}</WelcomeFactValue>
                <WelcomeFactLabel>sample files</WelcomeFactLabel>
              </WelcomeFact>
              <WelcomeFact>
                <WelcomeFactValue>{workspace.folderCount}</WelcomeFactValue>
                <WelcomeFactLabel>folders ready</WelcomeFactLabel>
              </WelcomeFact>
              <WelcomeFact>
                <WelcomeFactValue>{workspace.databaseName}</WelcomeFactValue>
                <WelcomeFactLabel>connected database</WelcomeFactLabel>
              </WelcomeFact>
            </WelcomeFacts>
            <WelcomeBody>
              Next, let&apos;s connect your first agent. Once it&apos;s linked, your agent can sync this workspace locally or use it through MCP.
            </WelcomeBody>
            <WelcomeActions>
              <Button
                size="large"
                onClick={() => {
                  setShowWhatsNext(true);
                  setTimeout(() => bannerStepRef.current?.jumpToStep(1), 0);
                }}
              >
                Connect my first agent
              </Button>
              <Button
                size="large"
                kind="ghost"
                onClick={() => {
                  void navigate({
                    to: "/workspaces/$workspaceId",
                    params: { workspaceId },
                    search: tab === "browse" ? {} : { tab },
                    replace: true,
                  });
                }}
              >
                I&apos;ll do this later
              </Button>
            </WelcomeActions>
          </WelcomeCard>
        </WelcomeInterstitial>
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

const WelcomeInterstitial = styled.section`
  display: flex;
  justify-content: center;
`;

const WelcomeCard = styled.div`
  width: min(100%, 760px);
  border-radius: 24px;
  padding: 28px;
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--afs-accent) 16%, transparent), transparent 28%),
    radial-gradient(circle at bottom left, rgba(255, 209, 102, 0.18), transparent 34%),
    linear-gradient(180deg, var(--afs-panel-strong), color-mix(in srgb, var(--afs-bg-soft) 58%, white));
  border: 1px solid color-mix(in srgb, var(--afs-accent) 16%, var(--afs-line));
  box-shadow: 0 24px 60px rgba(79, 51, 24, 0.10);

  @media (max-width: 720px) {
    padding: 22px;
  }
`;

const WelcomeHeader = styled.div`
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 10px;

  @media (max-width: 640px) {
    align-items: flex-start;
  }
`;

const WelcomeHeaderCopy = styled.div`
  min-width: 0;
`;

const WelcomeIconWrap = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: 16px;
  flex: 0 0 auto;
  background: linear-gradient(
    135deg,
    color-mix(in srgb, var(--afs-accent) 18%, white),
    color-mix(in srgb, #ffd98f 72%, white)
  );
  color: var(--afs-accent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--afs-accent) 14%, transparent);
`;

const WelcomeIconSpark = styled.span`
  font-size: 22px;
  line-height: 1;
`;

const WelcomeEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const WelcomeTitle = styled.h2`
  margin: 10px 0 12px;
  color: var(--afs-ink);
  font-size: clamp(28px, 4vw, 40px);
  line-height: 1.05;
`;

const WelcomeBody = styled.p`
  margin: 0;
  max-width: 66ch;
  color: var(--afs-muted);
  font-size: 16px;
  line-height: 1.6;

  & + & {
    margin-top: 10px;
  }
`;

const WelcomeWorkspaceName = styled.strong`
  color: var(--afs-ink);
`;

const WelcomeFacts = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  margin: 18px 0;

  @media (max-width: 820px) {
    grid-template-columns: 1fr;
  }
`;

const WelcomeFact = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 18px;
  padding: 14px 16px;
  background: color-mix(in srgb, var(--afs-panel) 72%, white);
`;

const WelcomeFactValue = styled.div`
  color: var(--afs-ink);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
  letter-spacing: -0.02em;
  word-break: break-word;
`;

const WelcomeFactLabel = styled.div`
  margin-top: 4px;
  color: var(--afs-muted);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
`;

const WelcomeActions = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 24px;
  flex-wrap: wrap;
`;
