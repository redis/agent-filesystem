import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useState } from "react";
import styled, { css, keyframes } from "styled-components";
import { z } from "zod";
import {
  DialogActions,
  DialogBody,
  DialogCard,
  DialogCloseButton,
  DialogError,
  DialogHeader,
  DialogOverlay,
  DialogTitle,
  EmptyState,
  NoticeBody,
  NoticeCard,
  NoticeTitle,
  PageStack,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { ConnectAgentBanner } from "../components/connect-agent-banner";
import { useAuthSession } from "../foundation/auth-context";
import {
  useDeleteWorkspaceMutation,
  useMCPAccessTokens,
  useUpdateWorkspaceMutation,
  useWorkspace,
  workspaceQueryOptions,
} from "../foundation/hooks/use-afs";
import { useDatabaseScope } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { resolveWorkspaceBrowserView } from "../foundation/workspace-browser-views";
import { displayWorkspaceName } from "../foundation/workspace-display";
import { normalizeStudioTab, studioTabSchema } from "../foundation/workspace-tabs";
import type { StudioTab } from "../foundation/workspace-tabs";
import type { AFSWorkspaceView } from "../foundation/types/afs";
import { BrowseTab } from "./workspace-studio/-browse-tab";
import { CheckpointsTab } from "./workspace-studio/-checkpoints-tab";
import { HistoryTab } from "./workspace-studio/-changes-tab";
import { SettingsTab } from "./workspace-studio/-settings-tab";

const workspaceStudioSearchSchema = z.object({
  tab: studioTabSchema.optional(),
  welcome: z.boolean().optional(),
  databaseId: z.string().optional(),
});

export const Route = createFileRoute("/workspaces/$workspaceId")({
  validateSearch: workspaceStudioSearchSchema,
  loader: ({ params, search }) =>
    queryClient.ensureQueryData({
      ...workspaceQueryOptions(search?.databaseId ?? null, params.workspaceId),
      revalidateIfStale: true,
    }),
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const auth = useAuthSession();
  const { workspaceId } = Route.useParams();
  const search = Route.useSearch();
  const databaseId = search.databaseId ?? null;
  const { unavailableDatabases } = useDatabaseScope();
  const workspaceQuery = useWorkspace(databaseId, workspaceId);
  const mcpAccessReady = !auth.isLoading && auth.isAuthenticated;
  const mcpTokensQuery = useMCPAccessTokens(databaseId, workspaceId, mcpAccessReady);
  const deleteWorkspace = useDeleteWorkspaceMutation();
  const updateWorkspace = useUpdateWorkspaceMutation();

  const [browserView, setBrowserView] = useState<AFSWorkspaceView>("head");
  const [bannerDismissed, setBannerDismissed] = useState(false);
  const [userRequestedBanner, setUserRequestedBanner] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [isRedirectingAfterDelete, setIsRedirectingAfterDelete] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const workspace = workspaceQuery.data;
  const tab = normalizeStudioTab(search.tab);
  const agentCount = workspace?.agents.length ?? 0;
  const hasAgents = agentCount > 0;
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

    setBrowserView((currentView) => resolveWorkspaceBrowserView(workspace, currentView));
  }, [workspace]);

  function setStudioTab(nextTab: StudioTab) {
    void navigate({
      to: "/workspaces/$workspaceId",
        params: { workspaceId },
        search: {
          ...(search.databaseId ? { databaseId: search.databaseId } : {}),
          ...(nextTab === "browse" ? {} : { tab: nextTab }),
        },
        replace: true,
    });
  }

  function deleteCurrentWorkspace() {
    if (workspace == null) {
      return;
    }
    setDeleteDialogOpen(true);
  }

  async function confirmDeleteCurrentWorkspace() {
    if (workspace == null) {
      return;
    }

    try {
      setDeleteDialogOpen(false);
      setIsRedirectingAfterDelete(true);
      await deleteWorkspace.mutateAsync({ databaseId: databaseId ?? undefined, workspaceId });
      await navigate({ to: "/workspaces", replace: true });
    } catch {
      setIsRedirectingAfterDelete(false);
      setDeleteDialogOpen(true);
      // keep the dialog open and show the mutation error below
    }
  }

  async function saveWorkspaceSettings(input: { name: string; description: string }) {
    if (workspace == null) {
      return;
    }

    try {
      setSaveError(null);
      await updateWorkspace.mutateAsync({
        databaseId: databaseId ?? undefined,
        workspaceId,
        name: input.name,
        description: input.description,
        cloudAccount: workspace.cloudAccount,
        databaseName: workspace.databaseName,
        region: workspace.region,
      });
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : "Unable to save workspace changes.");
    }
  }

  if (
    workspaceQuery.isLoading
    || isRedirectingAfterDelete
    || (workspaceQuery.isError && deleteWorkspace.isSuccess)
  ) {
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
    if (deleteWorkspace.isPending) {
      return <Loader data-testid="loader--spinner" />;
    }
    throw new Error("Workspace not found.");
  }

  const workspaceLabel = displayWorkspaceName(workspace.name);

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
                    search: {
                      ...(search.databaseId ? { databaseId: search.databaseId } : {}),
                      ...(tab === "browse" ? {} : { tab }),
                    },
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
                    search: {
                      ...(search.databaseId ? { databaseId: search.databaseId } : {}),
                      ...(tab === "browse" ? {} : { tab }),
                    },
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
          <BreadcrumbCurrent>{workspaceLabel}</BreadcrumbCurrent>
        </BreadcrumbGroup>
        <StudioActions>
          <AgentConnectionPill title={`${agentCount} connected agent${agentCount === 1 ? "" : "s"}`}>
            <AgentLiveDot $active={hasAgents} />
            {agentCount.toLocaleString()} connected agent{agentCount === 1 ? "" : "s"}
          </AgentConnectionPill>
          <ConnectAgentButton
            kind="ghost"
            size="large"
            onClick={() => {
              setUserRequestedBanner(true);
              setBannerDismissed(false);
            }}
          >
            Connect agent
          </ConnectAgentButton>
        </StudioActions>
      </StudioNavRow>

      <Tabs>
        <TabButton $active={tab === "browse"} onClick={() => setStudioTab("browse")}>
          Browse Files
        </TabButton>
        <TabButton $active={tab === "checkpoints"} onClick={() => setStudioTab("checkpoints")}>
          Checkpoints
        </TabButton>
        <TabButton $active={tab === "changes"} onClick={() => setStudioTab("changes")}>
          History
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
          onViewAllCheckpoints={() => setStudioTab("checkpoints")}
        />
      ) : null}

      {tab === "checkpoints" ? (
        <CheckpointsTab
          workspace={workspace}
          onBrowserViewChange={setBrowserView}
          onTabChange={setStudioTab}
        />
      ) : null}

      {tab === "changes" ? (
        <HistoryTab
          databaseId={workspace.databaseId}
          workspaceId={workspaceId}
        />
      ) : null}

      {tab === "settings" ? (
        <SettingsTab
          workspace={workspace}
          onSave={saveWorkspaceSettings}
          isSaving={updateWorkspace.isPending}
          saveError={saveError}
          onDelete={deleteCurrentWorkspace}
          isDeleting={deleteWorkspace.isPending}
          mcpTokens={mcpTokensQuery.data ?? []}
          onOpenMCPConsole={() => {
            void navigate({
              to: "/mcp",
              search: {
                workspaceId,
                ...(databaseId ? { databaseId } : {}),
              },
            });
          }}
        />
      ) : null}

      {deleteDialogOpen ? (
        <DialogOverlay
          role="dialog"
          aria-modal="true"
          aria-labelledby="delete-workspace-dialog-title"
          onClick={() => {
            if (!deleteWorkspace.isPending) {
              setDeleteDialogOpen(false);
            }
          }}
        >
          <ConfirmCard onClick={(event) => event.stopPropagation()}>
            <DialogHeader>
              <div>
                <DialogTitle id="delete-workspace-dialog-title">
                  Delete this workspace?
                </DialogTitle>
                <DialogBody>
                  Delete <strong>{workspaceLabel}</strong> and remove it from the workspace registry.
                  This action cannot be undone.
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={() => {
                  if (!deleteWorkspace.isPending) {
                    setDeleteDialogOpen(false);
                  }
                }}
              >
                ×
              </DialogCloseButton>
            </DialogHeader>

            {deleteWorkspace.error instanceof Error ? (
              <DialogError role="alert">{deleteWorkspace.error.message}</DialogError>
            ) : null}

            <DialogActions style={{ justifyContent: "flex-end", marginTop: 20 }}>
              <Button
                variant="secondary-fill"
                size="medium"
                onClick={() => setDeleteDialogOpen(false)}
                disabled={deleteWorkspace.isPending}
              >
                Cancel
              </Button>
              <DeleteConfirmButton
                size="medium"
                onClick={() => void confirmDeleteCurrentWorkspace()}
                disabled={deleteWorkspace.isPending}
              >
                {deleteWorkspace.isPending ? "Deleting..." : "Delete workspace"}
              </DeleteConfirmButton>
            </DialogActions>
          </ConfirmCard>
        </DialogOverlay>
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

  @media (max-width: 720px) {
    align-items: flex-start;
    flex-wrap: wrap;
  }
`;

const StudioActions = styled.div`
  display: flex;
  align-items: center;
  gap: 12px;

  @media (max-width: 720px) {
    width: 100%;
    justify-content: flex-start;
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

const ConnectAgentButton = styled(Button)`
  && {
    margin-left: auto;
    white-space: nowrap;
    box-shadow: none;
  }
`;

const agentPulse = keyframes`
  0%, 100% { opacity: 1; }
  50% { opacity: 0.45; }
`;

const AgentConnectionPill = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 34px;
  padding: 0;
  border: none;
  background: transparent;
  color: var(--afs-ink-soft);
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
`;

const AgentLiveDot = styled.span<{ $active: boolean }>`
  display: inline-block;
  width: 8px;
  height: 8px;
  flex-shrink: 0;
  border-radius: 50%;
  background: ${({ $active }) => ($active ? "#22c55e" : "#d1d5db")};
  ${({ $active }) =>
    $active &&
    css`
      box-shadow: 0 0 7px rgba(34, 197, 94, 0.65);
      animation: ${agentPulse} 2s ease-in-out infinite;
    `}
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

const ConfirmCard = styled(DialogCard)`
  max-width: 540px;
`;

const DeleteConfirmButton = styled(Button)`
  && {
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
