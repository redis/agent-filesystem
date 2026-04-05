import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redislabsdev/redis-ui-components";
import { useState } from "react";
import {
  Field,
  FormGrid,
  PageStack,
  SectionCard,
  SectionGrid,
  SectionHeader,
  SectionTitle,
  Select,
  TextArea,
  TextInput,
  TwoColumnFields,
} from "../components/raf-kit";
import { WorkspaceTable } from "../foundation/tables/workspace-table";
import {
  useCreateWorkspaceMutation,
  useWorkspaceSummaries,
} from "../foundation/hooks/use-raf";
import type { RAFWorkspaceSource } from "../foundation/types/raf";

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
  const [source, setSource] = useState<RAFWorkspaceSource>("blank");

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const workspaces = workspacesQuery.data ?? [];

  return (
    <PageStack>
      <SectionGrid>
        <SectionCard $span={4}>
          <SectionHeader>
            <SectionTitle
              eyebrow="Catalog"
              title="Create or import a workspace"
              body="This form now matches the control-plane shape more closely: a workspace record mapped to one backing database and AFS namespace."
            />
          </SectionHeader>

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
                placeholder="What this workspace is for and who owns it."
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
                onChange={(event) => setSource(event.target.value as RAFWorkspaceSource)}
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
        </SectionCard>

        <SectionCard $span={8}>
          <SectionHeader>
            <SectionTitle
              title="Workspace catalog"
              body="Each row is one Agent Filesystem. This table is shaped for the future cloud API and the matching CLI backend: database mapping, Redis key, content size, draft state, and checkpoints."
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
