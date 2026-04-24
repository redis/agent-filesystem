import { Button, Loader } from "@redis-ui/components";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import styled from "styled-components";
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
} from "../components/afs-kit";
import {
  useCreateMCPAccessTokenMutation,
  useDeleteWorkspaceMutation,
  useMCPAccessTokens,
  useRevokeMCPAccessTokenMutation,
  useWorkspace,
} from "../foundation/hooks/use-afs";
import { TemplateInstallDetail } from "../features/templates/TemplateInstallDetail";
import { findTemplate } from "../features/templates/templates-data";

const searchSchema = z.object({
  seeded: z.number().optional(),
  fresh: z.boolean().optional(),
  databaseId: z.string().optional(),
});

export const Route = createFileRoute("/templates/installed/$workspaceId")({
  validateSearch: searchSchema,
  component: TemplateInstallPage,
});

function TemplateInstallPage() {
  const navigate = useNavigate();
  const { workspaceId } = Route.useParams();
  const search = Route.useSearch() ?? {};
  const databaseId = search.databaseId ?? null;

  const workspaceQuery = useWorkspace(databaseId, workspaceId);
  const tokensQuery = useMCPAccessTokens(databaseId, workspaceId);

  const createToken = useCreateMCPAccessTokenMutation();
  const revokeToken = useRevokeMCPAccessTokenMutation();
  const deleteWorkspace = useDeleteWorkspaceMutation();

  const [confirmOpen, setConfirmOpen] = useState(false);
  const [uninstallError, setUninstallError] = useState<string | null>(null);
  const [isUninstalling, setIsUninstalling] = useState(false);
  const [repairError, setRepairError] = useState<string | null>(null);

  const workspace = workspaceQuery.data;
  const template = useMemo(
    () =>
      workspace?.templateSlug
        ? findTemplate(workspace.templateSlug)
        : undefined,
    [workspace?.templateSlug],
  );

  const activeTemplateTokens = useMemo(() => {
    if (!workspace?.templateSlug) return [];
    const tokens = tokensQuery.data ?? [];
    return tokens.filter(
      (token) =>
        token.templateSlug === workspace.templateSlug && !token.revokedAt,
    );
  }, [tokensQuery.data, workspace?.templateSlug]);

  const primaryToken = activeTemplateTokens[0] ?? null;

  async function repairToken() {
    if (!workspace || !template) return;
    try {
      setRepairError(null);
      await createToken.mutateAsync({
        databaseId: workspace.databaseId || undefined,
        workspaceId: workspace.id,
        name: `${template.title} setup`,
        profile: template.profile,
        templateSlug: template.id,
      });
    } catch (error) {
      setRepairError(
        error instanceof Error
          ? error.message
          : "Unable to recreate the access token.",
      );
    }
  }

  async function uninstall() {
    if (!workspace) return;
    setIsUninstalling(true);
    setUninstallError(null);
    try {
      for (const token of activeTemplateTokens) {
        await revokeToken.mutateAsync({
          databaseId: workspace.databaseId || undefined,
          workspaceId: workspace.id,
          tokenId: token.id,
        });
      }
      await deleteWorkspace.mutateAsync({
        databaseId: workspace.databaseId || undefined,
        workspaceId: workspace.id,
      });
      setConfirmOpen(false);
      await navigate({ to: "/templates", replace: true });
    } catch (error) {
      setUninstallError(
        error instanceof Error
          ? error.message
          : "Uninstall failed. Please retry.",
      );
    } finally {
      setIsUninstalling(false);
    }
  }

  if (workspaceQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  if (workspaceQuery.isError || workspace == null) {
    return (
      <PageStack>
        <EmptyState role="alert">
          <NoticeTitle>Workspace unavailable</NoticeTitle>
          <NoticeBody>
            This workspace could not be loaded. It may have been deleted.
          </NoticeBody>
          <NoticeBody>
            <Button
              size="medium"
              variant="secondary-fill"
              onClick={() => void navigate({ to: "/templates" })}
            >
              Back to templates
            </Button>
          </NoticeBody>
        </EmptyState>
      </PageStack>
    );
  }

  if (!workspace.templateSlug) {
    return (
      <PageStack>
        <EmptyState role="alert">
          <NoticeTitle>Not installed from a template</NoticeTitle>
          <NoticeBody>
            This workspace was created manually and has no template to
            revisit.
          </NoticeBody>
          <NoticeBody>
            <Button
              size="medium"
              onClick={() =>
                void navigate({
                  to: "/workspaces/$workspaceId",
                  params: { workspaceId: workspace.id },
                })
              }
            >
              Open workspace
            </Button>
          </NoticeBody>
        </EmptyState>
      </PageStack>
    );
  }

  if (!template) {
    return (
      <PageStack>
        <EmptyState role="alert">
          <NoticeTitle>Template no longer available</NoticeTitle>
          <NoticeBody>
            The template <code>{workspace.templateSlug}</code> is no longer
            registered. The workspace is still available.
          </NoticeBody>
          <NoticeBody>
            <Button
              size="medium"
              onClick={() =>
                void navigate({
                  to: "/workspaces/$workspaceId",
                  params: { workspaceId: workspace.id },
                })
              }
            >
              Open workspace
            </Button>
          </NoticeBody>
        </EmptyState>
      </PageStack>
    );
  }

  return (
    <Wrap>
      <Breadcrumb>
        <BreadcrumbLink
          type="button"
          onClick={() => void navigate({ to: "/templates" })}
        >
          <BreadcrumbArrow aria-hidden>&larr;</BreadcrumbArrow>
          Templates
        </BreadcrumbLink>
        <BreadcrumbSep aria-hidden>/</BreadcrumbSep>
        <BreadcrumbCurrent>{template.title}</BreadcrumbCurrent>
      </Breadcrumb>

      <Header>
        <IconSlot $accent={template.accent}>
          <template.icon size="M" />
        </IconSlot>
        <HeaderBody>
          <HeaderEyebrow>Installed template</HeaderEyebrow>
          <HeaderTitle>{template.title}</HeaderTitle>
          <HeaderTagline>
            Workspace <strong>{workspace.name}</strong> &middot;{" "}
            {workspace.fileCount} file{workspace.fileCount === 1 ? "" : "s"}
          </HeaderTagline>
        </HeaderBody>
        <HeaderLinks>
          <Button
            size="small"
            variant="secondary-fill"
            onClick={() =>
              void navigate({
                to: "/workspaces/$workspaceId",
                params: { workspaceId: workspace.id },
              })
            }
          >
            Open workspace
          </Button>
        </HeaderLinks>
      </Header>

      {primaryToken == null ? (
        <OrphanCard role="status">
          <NoticeTitle>Access token missing</NoticeTitle>
          <NoticeBody>
            This template's MCP token has been revoked or deleted. Recreate it
            to get a fresh bearer and restore connection instructions.
          </NoticeBody>
          {repairError ? (
            <DialogError role="alert">{repairError}</DialogError>
          ) : null}
          <div>
            <Button
              size="medium"
              disabled={createToken.isPending || tokensQuery.isLoading}
              onClick={() => {
                void repairToken();
              }}
            >
              {createToken.isPending
                ? "Recreating…"
                : "Recreate access token"}
            </Button>
          </div>
        </OrphanCard>
      ) : (
        <TemplateInstallDetail
          workspace={workspace}
          token={primaryToken}
          template={template}
          seededCount={search.seeded}
          showFreshBanner={search.fresh === true}
        />
      )}

      <DangerZone>
        <DangerZoneHeader>
          <DangerZoneTitle>Uninstall template</DangerZoneTitle>
          <DangerZoneDesc>
            Revokes this template's MCP token and deletes the workspace. All
            files stored in the workspace will be lost.
          </DangerZoneDesc>
        </DangerZoneHeader>
        <Button
          size="medium"
          variant="secondary-fill"
          onClick={() => {
            setUninstallError(null);
            setConfirmOpen(true);
          }}
        >
          Uninstall…
        </Button>
      </DangerZone>

      {confirmOpen ? (
        <DialogOverlay
          onClick={(event) => {
            if (event.target === event.currentTarget && !isUninstalling) {
              setConfirmOpen(false);
            }
          }}
        >
          <DialogCard>
            <DialogHeader>
              <div>
                <DialogTitle>Uninstall {template.title}?</DialogTitle>
                <DialogBody>
                  This removes the template's MCP access and permanently
                  deletes the workspace.
                </DialogBody>
              </div>
              <DialogCloseButton
                type="button"
                aria-label="Close"
                onClick={() => {
                  if (!isUninstalling) setConfirmOpen(false);
                }}
              >
                &times;
              </DialogCloseButton>
            </DialogHeader>
            <SummaryList>
              <SummaryRow>
                <SummaryLabel>Workspace</SummaryLabel>
                <SummaryValue>
                  <code>{workspace.name}</code>
                </SummaryValue>
              </SummaryRow>
              <SummaryRow>
                <SummaryLabel>Files</SummaryLabel>
                <SummaryValue>
                  {workspace.fileCount} across {workspace.folderCount} folder
                  {workspace.folderCount === 1 ? "" : "s"}
                </SummaryValue>
              </SummaryRow>
              <SummaryRow>
                <SummaryLabel>MCP tokens</SummaryLabel>
                <SummaryValue>
                  {activeTemplateTokens.length} active token
                  {activeTemplateTokens.length === 1 ? "" : "s"} will be
                  revoked
                </SummaryValue>
              </SummaryRow>
            </SummaryList>
            {uninstallError ? (
              <DialogError role="alert">{uninstallError}</DialogError>
            ) : null}
            <DialogActions style={{ justifyContent: "flex-end" }}>
              <Button
                size="medium"
                variant="secondary-fill"
                onClick={() => setConfirmOpen(false)}
                disabled={isUninstalling}
              >
                Cancel
              </Button>
              <Button
                size="medium"
                onClick={() => {
                  void uninstall();
                }}
                disabled={isUninstalling}
              >
                {isUninstalling
                  ? "Uninstalling…"
                  : "Uninstall and delete"}
              </Button>
            </DialogActions>
          </DialogCard>
        </DialogOverlay>
      ) : null}
    </Wrap>
  );
}

const Wrap = styled.div`
  display: flex;
  flex-direction: column;
  gap: 24px;
  width: min(100%, 880px);
  margin: 0 auto;
  padding: 28px 32px 56px;

  @media (max-width: 900px) {
    padding: 20px 18px 40px;
  }
`;

const Breadcrumb = styled.nav`
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--afs-muted);
  font-size: 13px;
`;

const BreadcrumbLink = styled.button`
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 6px;
  margin-left: -6px;
  border: none;
  background: transparent;
  color: var(--afs-muted);
  font-size: 13px;
  font-weight: 600;
  border-radius: 8px;
  cursor: pointer;
  transition: color 120ms ease, background 120ms ease;

  &:hover {
    color: var(--afs-accent, #2563eb);
    background: color-mix(in srgb, var(--afs-muted) 8%, transparent);
  }
`;

const BreadcrumbArrow = styled.span`
  font-size: 14px;
  line-height: 1;
`;

const BreadcrumbSep = styled.span`
  color: var(--afs-line);
`;

const BreadcrumbCurrent = styled.span`
  color: var(--afs-ink);
  font-weight: 600;
`;

const Header = styled.header`
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 16px 18px;
  border: 1px solid var(--afs-line);
  border-radius: 14px;
  background: var(--afs-panel-strong);
`;

const IconSlot = styled.div<{ $accent: string }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: ${({ $accent }) =>
    `color-mix(in srgb, ${$accent} 18%, transparent)`};
  color: ${({ $accent }) => $accent};
  flex-shrink: 0;
`;

const HeaderBody = styled.div`
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const HeaderEyebrow = styled.span`
  font-size: 10.5px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--afs-muted);
`;

const HeaderTitle = styled.h1`
  margin: 0;
  font-size: 18px;
  font-weight: 700;
  color: var(--afs-ink);
  letter-spacing: -0.01em;
`;

const HeaderTagline = styled.p`
  margin: 2px 0 0;
  color: var(--afs-muted);
  font-size: 13px;

  strong {
    color: var(--afs-ink);
    font-weight: 700;
  }
`;

const HeaderLinks = styled.div`
  display: flex;
  gap: 8px;
  flex-shrink: 0;
`;

const OrphanCard = styled(NoticeCard)`
  display: flex;
  flex-direction: column;
  gap: 10px;
`;

const DangerZone = styled.div`
  display: flex;
  align-items: center;
  gap: 14px;
  justify-content: space-between;
  padding: 14px 16px;
  border: 1px dashed color-mix(in srgb, #dc2626 40%, var(--afs-line));
  border-radius: 12px;
  background: color-mix(in srgb, #dc2626 6%, transparent);

  @media (max-width: 560px) {
    flex-direction: column;
    align-items: stretch;
  }
`;

const DangerZoneHeader = styled.div`
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
`;

const DangerZoneTitle = styled.h3`
  margin: 0;
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink);
`;

const DangerZoneDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.5;
`;

const SummaryList = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 8px 0 12px;
  padding: 12px 14px;
  border: 1px solid var(--afs-line);
  border-radius: 10px;
  background: var(--afs-panel);
`;

const SummaryRow = styled.div`
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
`;

const SummaryLabel = styled.span`
  color: var(--afs-muted);
  font-weight: 600;
`;

const SummaryValue = styled.span`
  color: var(--afs-ink);
  text-align: right;

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 12px;
    padding: 1px 6px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--afs-line) 60%, transparent);
  }
`;
