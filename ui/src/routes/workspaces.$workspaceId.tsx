import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { useEffect, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import {
  PageStack,
  SectionCard,
  SectionGrid,
  TabButton,
  Tabs,
} from "../components/afs-kit";
import { useDatabaseScope } from "../foundation/database-scope";
import {
  useDeleteWorkspaceMutation,
  useWorkspace,
} from "../foundation/hooks/use-afs";
import type { AFSWorkspaceView } from "../foundation/types/afs";
import { OverviewTab } from "./workspace-studio/-overview-tab";
import { FilesTab } from "./workspace-studio/-files-tab";
import { CheckpointsTab } from "./workspace-studio/-checkpoints-tab";
import { ActivityTab } from "./workspace-studio/-activity-tab";

type StudioTab = "overview" | "files" | "checkpoints" | "activity";

const workspaceStudioSearchSchema = z.object({
  tab: z.enum(["overview", "files", "checkpoints", "activity"]).optional(),
});

export const Route = createFileRoute("/workspaces/$workspaceId")({
  validateSearch: workspaceStudioSearchSchema,
  component: WorkspaceStudioPage,
});

function WorkspaceStudioPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const search = Route.useSearch();
  const { selectedDatabaseId } = useDatabaseScope();
  const workspaceQuery = useWorkspace(selectedDatabaseId, workspaceId);
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [browserView, setBrowserView] = useState<AFSWorkspaceView>("head");

  const workspace = workspaceQuery.data;
  const tab = search.tab ?? "overview";

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
      search: nextTab === "overview" ? {} : { tab: nextTab },
      replace: true,
    });
  }

  function deleteCurrentWorkspace() {
    const confirmed = window.confirm(
      `Delete workspace "${workspace?.name ?? workspaceId}"? This removes it from the workspace registry.`,
    );

    if (!confirmed) {
      return;
    }

    deleteWorkspace.mutate({
      databaseId: selectedDatabaseId ?? "",
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

  if (selectedDatabaseId == null) {
    throw new Error("No database selected.");
  }

  if (workspace == null) {
    throw new Error("Workspace not found.");
  }

  return (
    <PageStack>
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
        <DeleteWorkspaceButton
          size="large"
          disabled={deleteWorkspace.isPending}
          onClick={deleteCurrentWorkspace}
        >
          {deleteWorkspace.isPending ? "Deleting..." : "Delete workspace"}
        </DeleteWorkspaceButton>
      </StudioNavRow>

      <SectionGrid>
        <SectionCard $span={12}>
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
        </SectionCard>
      </SectionGrid>

      {tab === "overview" ? <OverviewTab workspace={workspace} /> : null}

      {tab === "files" ? (
        <FilesTab
          workspace={workspace}
          databaseId={selectedDatabaseId}
          browserView={browserView}
          onBrowserViewChange={setBrowserView}
        />
      ) : null}

      {tab === "checkpoints" ? (
        <CheckpointsTab
          workspace={workspace}
          databaseId={selectedDatabaseId}
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
  color: var(--afs-muted);
  font-size: 14px;
  font-weight: 500;
`;

const DeleteWorkspaceButton = styled(Button)`
  && {
    white-space: nowrap;
    background: ${({ theme }) => theme.semantic.color.background.danger500};
    border-color: ${({ theme }) => theme.semantic.color.background.danger500};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }

  &&:hover:not(:disabled),
  &&:focus-visible:not(:disabled) {
    background: ${({ theme }) => theme.semantic.color.background.danger600};
    border-color: ${({ theme }) => theme.semantic.color.background.danger600};
    color: ${({ theme }) => theme.semantic.color.text.inverse};
  }
`;
