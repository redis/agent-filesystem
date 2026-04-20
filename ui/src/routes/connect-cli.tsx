import { createFileRoute, Link } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useEffect, useMemo, useRef, useState } from "react";
import styled from "styled-components";
import { z } from "zod";
import { afsApi } from "../foundation/api/afs";
import { useDatabaseScope } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import { workspaceSummariesQueryOptions, useWorkspaceSummaries } from "../foundation/hooks/use-afs";

const connectCLISearchSchema = z.object({
  return_to: z.string().url(),
  state: z.string().min(1),
  workspace: z.string().optional(),
  connected: z.boolean().optional(),
  workspace_name: z.string().optional(),
});

export const Route = createFileRoute("/connect-cli")({
  validateSearch: connectCLISearchSchema,
  loader: async () => {
    await queryClient.ensureQueryData({
      ...workspaceSummariesQueryOptions(null),
      revalidateIfStale: true,
    });
  },
  component: ConnectCLIPage,
});

function ConnectCLIPage() {
  const search = Route.useSearch();
  const workspacesQuery = useWorkspaceSummaries(null);
  const { databases } = useDatabaseScope();
  const [error, setError] = useState<string | null>(null);
  const [connectingWorkspaceId, setConnectingWorkspaceId] = useState<string | null>(null);
  const autoConnectAttempted = useRef(false);

  const workspaces = workspacesQuery.data ?? [];
  const hasCreatableDatabase = databases.some((database) => database.canCreateWorkspaces);
  const returnToError = validateReturnTo(search.return_to);
  const autoWorkspace = useMemo(() => {
    const workspaceHint = search.workspace?.trim();
    if (workspaceHint) {
      const hinted = workspaces.find((workspace) => workspace.id === workspaceHint || workspace.name === workspaceHint);
      if (hinted != null) {
        return hinted;
      }
    }

    const gettingStarted = workspaces.find((workspace) => isGettingStartedName(workspace.name));
    if (gettingStarted != null) {
      return gettingStarted;
    }
    if (workspaces.length === 1) {
      return workspaces[0];
    }
    return null;
  }, [search.workspace, workspaces]);
  const explicitWorkspaceHint = search.workspace?.trim() ?? "";

  async function redirectWithOnboardingToken(createToken: () => Promise<{ token: string; workspaceName: string }>) {
    if (returnToError != null) {
      setError(returnToError);
      return;
    }

    try {
      const onboarding = await createToken();
      const target = new URL(search.return_to);
      target.searchParams.set("token", onboarding.token);
      target.searchParams.set("state", search.state);
      target.searchParams.set("workspace", onboarding.workspaceName);
      window.location.assign(target.toString());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not prepare the CLI login.");
    }
  }

  async function connectWorkspace(workspaceId: string, databaseId: string) {
    setError(null);
    setConnectingWorkspaceId(workspaceId);
    await redirectWithOnboardingToken(() => afsApi.createOnboardingToken(databaseId, workspaceId));
  }

  useEffect(() => {
    if (autoConnectAttempted.current || autoWorkspace == null || workspacesQuery.isLoading || returnToError != null) {
      return;
    }
    autoConnectAttempted.current = true;
    void connectWorkspace(autoWorkspace.id, autoWorkspace.databaseId);
  }, [autoWorkspace, returnToError, workspacesQuery.isLoading]);

  useEffect(() => {
    if (
      autoConnectAttempted.current ||
      explicitWorkspaceHint === "" ||
      autoWorkspace != null ||
      workspacesQuery.isLoading ||
      returnToError != null
    ) {
      return;
    }
    autoConnectAttempted.current = true;
    setConnectingWorkspaceId(explicitWorkspaceHint);
    setError(null);
    void redirectWithOnboardingToken(() => afsApi.createOnboardingToken(undefined, explicitWorkspaceHint));
  }, [autoWorkspace, explicitWorkspaceHint, returnToError, workspacesQuery.isLoading]);

  if (workspacesQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (search.connected) {
    return (
      <PageShell>
        <ConnectCard>
          <Eyebrow>AFS Cloud</Eyebrow>
          <Title>CLI connected</Title>
          <Description>
            {search.workspace_name?.trim()
              ? `Your terminal is now linked to ${search.workspace_name}. Return to the terminal and run afs up to sync it locally.`
              : "Your terminal is now linked. Return to the terminal and run afs up to sync the workspace locally."}
          </Description>
          <SuccessPanel>
            <SuccessCode>afs up</SuccessCode>
            <SuccessHint>You can close this tab after the terminal finishes starting the workspace.</SuccessHint>
          </SuccessPanel>
        </ConnectCard>
      </PageShell>
    );
  }

  return (
    <PageShell>
      <ConnectCard>
        <Eyebrow>AFS Cloud</Eyebrow>
        <Title>Connect your CLI</Title>
        <Description>
          {autoWorkspace == null
            ? "Pick a workspace for this terminal. If you have not created one yet, finish onboarding in AFS Cloud first."
            : isGettingStartedName(autoWorkspace.name)
              ? "Connecting this CLI to your getting-started workspace so you can start with sample files right away."
              : `Preparing browser login for ${autoWorkspace.name}.`}
        </Description>

        {returnToError != null ? (
          <InlineError>{returnToError}</InlineError>
        ) : null}
        {error != null ? (
          <InlineError>{error}</InlineError>
        ) : null}

        {workspaces.length === 0 ? (
          <CreateAnotherSection>
            <SectionHeading>{explicitWorkspaceHint ? "Preparing your starter workspace…" : "No workspaces yet"}</SectionHeading>
            <SectionCopy>
              {explicitWorkspaceHint
                ? "AFS Cloud is trying to connect this CLI to your starter workspace. If this page stays here, refresh once and retry the login from the terminal."
                : hasCreatableDatabase
                ? "Create your first workspace in AFS Cloud, then come back here to connect this CLI."
                : "Your account only has access to the shared getting-started database right now. Create your own cloud database or add an existing Redis database before creating a workspace."}
            </SectionCopy>
            {explicitWorkspaceHint ? (
              <LoadingPanel>
                <Loader data-testid="loader--spinner" />
                <p>Looking for {explicitWorkspaceHint}…</p>
              </LoadingPanel>
            ) : (
              <Link to={hasCreatableDatabase ? "/workspaces" : "/databases"}>
                <Button size="large" variant="secondary-fill">
                  {hasCreatableDatabase ? "Open workspace manager" : "Open database manager"}
                </Button>
              </Link>
            )}
          </CreateAnotherSection>
        ) : autoWorkspace != null && connectingWorkspaceId === autoWorkspace.id ? (
          <LoadingPanel>
            <Loader data-testid="loader--spinner" />
            <p>{isGettingStartedName(autoWorkspace.name) ? "Connecting you to getting-started…" : "Finishing your CLI login…"}</p>
          </LoadingPanel>
        ) : (
          <>
            <WorkspaceList>
              {workspaces.map((workspace) => (
                <WorkspaceRow key={workspace.id}>
                  <WorkspaceMeta>
                    <WorkspaceName>{workspace.name}</WorkspaceName>
                    <WorkspaceDetails>
                      {workspace.databaseName} · {workspace.fileCount} files · {workspace.checkpointCount} checkpoints
                    </WorkspaceDetails>
                  </WorkspaceMeta>
                  <Button
                    size="large"
                    disabled={connectingWorkspaceId != null}
                    onClick={() => {
                      void connectWorkspace(workspace.id, workspace.databaseId);
                    }}
                  >
                    {connectingWorkspaceId === workspace.id ? "Connecting..." : "Connect"}
                  </Button>
                </WorkspaceRow>
              ))}
            </WorkspaceList>

            <CreateAnotherSection>
              <SectionHeading>Want something other than getting-started?</SectionHeading>
              <SectionCopy>Create or import another workspace in AFS Cloud, then come back here and connect it.</SectionCopy>
              <Link to="/workspaces">
                <Button size="large" variant="secondary-fill">Open workspace manager</Button>
              </Link>
            </CreateAnotherSection>
          </>
        )}
      </ConnectCard>
    </PageShell>
  );
}

function isGettingStartedName(name: string) {
  return name === "getting-started" || name.startsWith("getting-started-");
}

function validateReturnTo(raw: string) {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return "The CLI did not provide a valid return URL.";
  }

  if (parsed.protocol !== "http:") {
    return "The CLI return URL must use http://localhost.";
  }
  const hostname = parsed.hostname.trim().toLowerCase();
  if (hostname !== "127.0.0.1" && hostname !== "localhost") {
    return "The CLI return URL must target localhost.";
  }
  return null;
}

const PageShell = styled.div`
  min-height: calc(100vh - 120px);
  display: grid;
  place-items: center;
  padding: 32px 0 48px;
`;

const ConnectCard = styled.section`
  width: min(760px, 100%);
  background: linear-gradient(180deg, rgba(255, 252, 244, 0.96), rgba(250, 245, 232, 0.96));
  border: 1px solid rgba(161, 134, 70, 0.18);
  border-radius: 28px;
  box-shadow: 0 24px 60px rgba(74, 56, 22, 0.1);
  padding: 32px;
`;

const Eyebrow = styled.div`
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: #8a6a1f;
  margin-bottom: 14px;
`;

const Title = styled.h1`
  margin: 0;
  font-size: clamp(32px, 5vw, 48px);
  line-height: 1;
  letter-spacing: -0.04em;
  color: #1d170b;
`;

const Description = styled.p`
  margin: 14px 0 0;
  font-size: 17px;
  line-height: 1.6;
  color: #5f533d;
`;

const WorkspaceList = styled.div`
  display: grid;
  gap: 14px;
  margin-top: 28px;
`;

const WorkspaceRow = styled.div`
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  border: 1px solid rgba(161, 134, 70, 0.16);
  border-radius: 18px;
  padding: 18px 20px;
  background: rgba(255, 255, 255, 0.82);
`;

const WorkspaceMeta = styled.div`
  min-width: 0;
`;

const WorkspaceName = styled.div`
  font-size: 20px;
  font-weight: 700;
  color: #1d170b;
`;

const WorkspaceDetails = styled.div`
  margin-top: 6px;
  font-size: 14px;
  line-height: 1.5;
  color: #72654a;
`;

const InlineError = styled.div`
  margin-top: 20px;
  padding: 14px 16px;
  border-radius: 14px;
  background: rgba(191, 50, 31, 0.08);
  color: #8f2210;
  font-size: 14px;
  line-height: 1.6;
`;

const LoadingPanel = styled.div`
  display: grid;
  gap: 12px;
  justify-items: center;
  margin-top: 32px;

  p {
    margin: 0;
    color: #5f533d;
  }
`;

const SectionHeading = styled.h2`
  margin: 0;
  font-size: 24px;
  line-height: 1.1;
  color: #1d170b;
`;

const SectionCopy = styled.p`
  margin: 10px 0 0;
  font-size: 15px;
  line-height: 1.6;
  color: #5f533d;
`;

const SuccessPanel = styled.div`
  display: grid;
  gap: 12px;
  margin-top: 28px;
  padding: 20px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.82);
  border: 1px solid rgba(161, 134, 70, 0.16);
`;

const SuccessCode = styled.code`
  display: inline-block;
  font-size: 24px;
  font-weight: 700;
  color: #1d170b;
  background: rgba(248, 241, 221, 0.9);
  border-radius: 12px;
  padding: 12px 14px;
`;

const SuccessHint = styled.p`
  margin: 0;
  color: #5f533d;
  line-height: 1.6;
`;

const CreateAnotherSection = styled.div`
  display: grid;
  gap: 10px;
  margin-top: 28px;
  padding-top: 24px;
  border-top: 1px solid rgba(161, 134, 70, 0.16);
`;
