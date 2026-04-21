import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useState } from "react";
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
import {
  useDeleteWorkspaceMutation,
  useWorkspace,
  workspaceQueryOptions,
} from "../foundation/hooks/use-afs";
import { useDatabaseScope } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { studioTabSchema } from "../foundation/workspace-tabs";
import type { StudioTab } from "../foundation/workspace-tabs";
import type { AFSWorkspaceView } from "../foundation/types/afs";
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
  const [userRequestedBanner, setUserRequestedBanner] = useState(false);

  const workspace = workspaceQuery.data;
  const tab = search.tab ?? "browse";
  const hasAgents = (workspace?.agents.length ?? 0) > 0;
  const showBanner = workspace != null && !bannerDismissed && userRequestedBanner;
  const showWelcomeInterstitial =
    workspace != null && search.welcome === true && !userRequestedBanner;

  // Always poll while on this page so we detect agent connections promptly.
  useEffect(() => {
    const interval = setInterval(() => {
      void workspaceQuery.refetch();
    }, 5000);
    return () => clearInterval(interval);
  }, []);

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

      {showWelcomeInterstitial ? (
        <WelcomeInterstitial>
          <WelcomeCard>
            <WelcomeEyebrow>Step 1 of 2</WelcomeEyebrow>
            <WelcomeTitle>Workspace Created!</WelcomeTitle>
            <WorkspaceChip>
              <ChipDot />
              <ChipName>{workspaceLabel}</ChipName>
            </WorkspaceChip>
            <WelcomeBody>
              We loaded your new workspace with sample files so you can
              explore AFS immediately.
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
            </WelcomeFacts>
            <WelcomeBody>
              Next, connect your first agent. Once linked, it can sync this
              workspace locally or access it through MCP.
            </WelcomeBody>
            <WelcomeActions>
              <Button
                size="large"
                variant="secondary-fill"
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
              <Button
                size="large"
                onClick={() => {
                  setUserRequestedBanner(true);
                  setBannerDismissed(false);
                  // Hide the interstitial so the banner takes focus.
                  void navigate({
                    to: "/workspaces/$workspaceId",
                    params: { workspaceId },
                    search: tab === "browse" ? {} : { tab },
                    replace: true,
                  });
                }}
              >
                Connect my first agent &rarr;
              </Button>
            </WelcomeActions>
          </WelcomeCard>
        </WelcomeInterstitial>
      ) : null}

      {showBanner ? (
        <ConnectAgentBanner
          workspaceId={workspaceId}
          workspaceName={workspace.name}
          workspaceLabel={workspaceLabel}
          agentConnected={hasAgents}
          onDismiss={() => {
            setBannerDismissed(true);
            setUserRequestedBanner(false);
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

const WelcomeEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const WelcomeTitle = styled.h2`
  margin: 10px 0 16px;
  color: var(--afs-ink);
  font-size: clamp(28px, 4vw, 38px);
  line-height: 1.08;
  letter-spacing: -0.02em;
`;

const WorkspaceChip = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px 6px 10px;
  border-radius: 999px;
  background: #ecfdf5;
  color: #047857;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 18px;
`;

const ChipDot = styled.span`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18);
`;

const ChipName = styled.span`
  color: #065f46;
`;

const WelcomeBody = styled.p`
  margin: 0;
  max-width: 60ch;
  color: var(--afs-muted);
  font-size: 15px;
  line-height: 1.6;

  & + & {
    margin-top: 10px;
  }
`;

const WelcomeFacts = styled.div`
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  margin: 20px 0;

  @media (max-width: 520px) {
    grid-template-columns: 1fr;
  }
`;

const WelcomeFact = styled.div`
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  padding: 14px 16px;
  background: color-mix(in srgb, var(--afs-panel) 72%, white);
`;

const WelcomeFactValue = styled.div`
  color: var(--afs-ink);
  font-size: 20px;
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
  justify-content: space-between;
  gap: 12px;
  margin-top: 28px;
  flex-wrap: wrap;
`;
