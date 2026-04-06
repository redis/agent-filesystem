import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { useState } from "react";
import styled from "styled-components";
import {
  Field,
  FormGrid,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  Select,
  StatCard,
  StatDetail,
  StatGrid,
  StatLabel,
  StatValue,
  TextArea,
  TextInput,
  TwoColumnFields,
} from "../components/afs-kit";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import {
  useCreateWorkspaceMutation,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-afs";
import type { AFSWorkspaceSource } from "../foundation/types/afs";

export const Route = createFileRoute("/workspaces")({
  component: WorkspacesPage,
});

function WorkspacesPage() {
  const navigate = useNavigate();
  const workspacesQuery = useWorkspaceSummaries();
  const createWorkspace = useCreateWorkspaceMutation();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [cloudAccount, setCloudAccount] = useState("Redis Cloud / Product");
  const [databaseName, setDatabaseName] = useState("agentfs-dev-us-east-1");
  const [region, setRegion] = useState("us-east-1");
  const [source, setSource] = useState<AFSWorkspaceSource>("blank");

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data ?? [];
  const healthy = workspaces.filter((workspace) => workspace.status === "healthy").length;
  const dirty = workspaces.filter((workspace) => workspace.draftState === "dirty").length;
  const imported = workspaces.filter((workspace) => workspace.source !== "blank").length;
  const checkpoints = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);

  return (
    <PageStack>
      <StatGrid>
        <StatCard>
          <div>
            <StatLabel>Catalog Size</StatLabel>
            <StatValue>{workspaces.length}</StatValue>
          </div>
          <StatDetail>Each row is one complete Redis-backed filesystem namespace.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Healthy</StatLabel>
            <StatValue>{healthy}</StatValue>
          </div>
          <StatDetail>Healthy workspaces are ready to browse, edit, or checkpoint.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Dirty Drafts</StatLabel>
            <StatValue>{dirty}</StatValue>
          </div>
          <StatDetail>Dirty working copies deserve fast access from the catalog.</StatDetail>
        </StatCard>
        <StatCard>
          <div>
            <StatLabel>Imported</StatLabel>
            <StatValue>{imported}</StatValue>
          </div>
          <StatDetail>{checkpoints} checkpoints are already attached across imported and local workspaces.</StatDetail>
        </StatCard>
      </StatGrid>

      <SectionGrid>
        <SectionCard $span={4}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Workspace Intake"
              title="Create or import a workspace"
              body="This intake surface should feel closer to provisioning a managed environment than filling out a generic form. Source choice, database mapping, and region belong together."
            />
          </SectionHeader>

          <SourcePicker>
            <SourceOption
              $active={source === "blank"}
              type="button"
              onClick={() => setSource("blank")}
            >
              <SourceTitle>Blank workspace</SourceTitle>
              <SourceBody>Start with an empty filesystem and build fresh state in Redis.</SourceBody>
            </SourceOption>
            <SourceOption
              $active={source === "git-import"}
              type="button"
              onClick={() => setSource("git-import")}
            >
              <SourceTitle>Git import</SourceTitle>
              <SourceBody>Seed a workspace from code or configuration that still needs browser-side edits.</SourceBody>
            </SourceOption>
            <SourceOption
              $active={source === "cloud-import"}
              type="button"
              onClick={() => setSource("cloud-import")}
            >
              <SourceTitle>Redis Cloud import</SourceTitle>
              <SourceBody>Pull an existing managed workspace into the control plane catalog.</SourceBody>
            </SourceOption>
          </SourcePicker>

          <FormGrid
            onSubmit={(event) => {
              event.preventDefault();
              if (name.trim() === "") {
                return;
              }

              createWorkspace.mutate(
                {
                  name,
                  description,
                  cloudAccount,
                  databaseName,
                  region,
                  source,
                },
                {
                  onSuccess: (workspace) => {
                    setName("");
                    setDescription("");
                    setDatabaseName("agentfs-dev-us-east-1");
                    void navigate({
                      to: "/workspaces/$workspaceId",
                      params: { workspaceId: workspace.id },
                    });
                  },
                },
              );
            }}
          >
            <TwoColumnFields>
              <Field>
                Workspace name
                <TextInput
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="customer-portal"
                />
              </Field>
              <Field>
                Database name
                <TextInput
                  value={databaseName}
                  onChange={(event) => setDatabaseName(event.target.value)}
                  placeholder="agentfs-dev-us-east-1"
                />
              </Field>
            </TwoColumnFields>
            <Field>
              Description
              <TextArea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="What this workspace is for, which team owns it, and why it exists."
              />
            </Field>
            <TwoColumnFields>
              <Field>
                Cloud account
                <TextInput
                  value={cloudAccount}
                  onChange={(event) => setCloudAccount(event.target.value)}
                />
              </Field>
              <Field>
                Region
                <TextInput
                  value={region}
                  onChange={(event) => setRegion(event.target.value)}
                />
              </Field>
            </TwoColumnFields>
            <Field>
              Source
              <Select
                value={source}
                onChange={(event) => setSource(event.target.value as AFSWorkspaceSource)}
              >
                <option value="blank">Blank workspace</option>
                <option value="git-import">Git import</option>
                <option value="cloud-import">Redis Cloud import</option>
              </Select>
            </Field>
            <Button disabled={createWorkspace.isPending} size="medium" type="submit">
              {createWorkspace.isPending ? "Creating..." : "Create workspace"}
            </Button>
          </FormGrid>

          <FormNote>
            Opening a workspace immediately after creation keeps the catalog action-oriented: provision
            here, inspect in the studio, then checkpoint once the draft settles.
          </FormNote>
        </SectionCard>

        <SectionCard $span={8}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Fleet Catalog"
              title="Workspace catalog"
              body="The table should support triage as much as discovery: status, draft state, content size, and checkpoint depth all matter before you open the studio."
            />
          </SectionHeader>

          <WorkspaceTable
            rows={workspaces}
            loading={workspacesQuery.isLoading}
            error={workspacesQuery.isError}
            onOpenWorkspace={(workspaceId) =>
              void navigate({
                to: "/workspaces/$workspaceId",
                params: { workspaceId },
              })
            }
          />
        </SectionCard>
      </SectionGrid>
    </PageStack>
  );
}

const SourcePicker = styled.div`
  display: grid;
  gap: 10px;
  margin-bottom: 18px;
`;

const SourceOption = styled.button<{ $active: boolean }>`
  border: 1px solid
    ${({ $active }) => ($active ? "var(--afs-line-strong)" : "var(--afs-line)")};
  border-radius: 18px;
  padding: 14px 15px;
  background: ${({ $active }) =>
    $active ? "var(--afs-accent-soft)" : "rgba(255, 255, 255, 0.74)"};
  text-align: left;
  cursor: pointer;
  transition:
    transform 160ms ease,
    border-color 160ms ease,
    background 160ms ease;

  &:hover {
    transform: translateY(-1px);
  }
`;

const SourceTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
`;

const SourceBody = styled.p`
  margin: 6px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const FormNote = styled.p`
  margin: 18px 0 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.7;
`;
