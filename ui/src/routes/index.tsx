import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { Button, Loader } from "@redis-ui/components";
import { useState } from "react";
import styled from "styled-components";
import {
  PageStack,
  StatCard,
  StatGrid,
  StatDetail,
  StatLabel,
  StatValue,
} from "../components/afs-kit";
import { AgentHeroAnimation } from "../components/agent-hero-animation";
import { GettingStartedOnboardingDialog } from "../components/getting-started-onboarding-dialog";
import { LiveTopologyCard } from "../components/live-topology-card";
import { CreateWorkspaceDialog } from "../features/workspaces/CreateWorkspaceDialog";
import { formatBytes } from "../foundation/api/afs";
import { useDatabaseScope, useScopedAgents, useScopedWorkspaceSummaries } from "../foundation/database-scope";
import { queryClient } from "../foundation/query-client";
import {
  agentsQueryOptions,
  databasesQueryOptions,
  useQuickstartMutation,
  workspaceSummariesQueryOptions,
} from "../foundation/hooks/use-afs";
import type { AFSWorkspaceDetail } from "../foundation/types/afs";
import { templates } from "../features/templates/templates-data";

const FEATURED_TEMPLATES = templates.filter((template) => template.id !== "blank").slice(0, 4);

export const Route = createFileRoute("/")({
  loader: async () => {
    await Promise.all([
      queryClient.ensureQueryData({ ...databasesQueryOptions(), revalidateIfStale: true }),
      queryClient.ensureQueryData({
        ...workspaceSummariesQueryOptions(null),
        revalidateIfStale: true,
      }),
      queryClient.ensureQueryData({ ...agentsQueryOptions(null), revalidateIfStale: true }),
    ]);
  },
  component: OverviewPage,
});

function OverviewPage() {
  const workspacesQuery = useScopedWorkspaceSummaries();
  const agentsQuery = useScopedAgents();
  const { databases, isLoading: databasesLoading } = useDatabaseScope();
  const [onboardingWorkspace, setOnboardingWorkspace] = useState<AFSWorkspaceDetail | null>(null);

  if (databasesLoading || workspacesQuery.isLoading || agentsQuery.isLoading) {
    return <Loader data-testid="loader--spinner" />;
  }

  const hasDatabase = databases.length > 0;

  const workspaces = workspacesQuery.data;
  let content;
  if (!hasDatabase) {
    content = <GettingStartedView hasDatabase={false} onQuickstartCreated={setOnboardingWorkspace} />;
  } else if (workspaces.length === 0) {
    content = <GettingStartedView hasDatabase={true} onQuickstartCreated={setOnboardingWorkspace} />;
  } else {
    /* ── Dashboard ── */
    const workspacesWithCheckpoints = workspaces.filter((workspace) => workspace.checkpointCount > 0).length;
    const checkpointCount = workspaces.reduce((sum, workspace) => sum + workspace.checkpointCount, 0);
    const totalBytes = workspaces.reduce((sum, workspace) => sum + workspace.totalBytes, 0);
    const checkpointCoverage = workspaces.length === 0 ? 0 : Math.round((workspacesWithCheckpoints / workspaces.length) * 100);

    content = (
      <DashboardView
        databases={databases}
        workspaces={workspaces}
        agents={agentsQuery.data}
        checkpointCount={checkpointCount}
        checkpointCoverage={checkpointCoverage}
        totalBytes={totalBytes}
      />
    );
  }

  return (
    <>
      {content}
      {onboardingWorkspace ? (
        <GettingStartedOnboardingDialog
          open
          workspaceId={onboardingWorkspace.id}
          workspaceName={onboardingWorkspace.name}
          databaseName={onboardingWorkspace.databaseName}
          fileCount={onboardingWorkspace.fileCount}
          folderCount={onboardingWorkspace.folderCount}
          onClose={() => setOnboardingWorkspace(null)}
        />
      ) : null}
    </>
  );
}

function DashboardView({ databases, workspaces, agents, checkpointCount, checkpointCoverage, totalBytes }: {
  databases: { length: number };
  workspaces: { length: number }[];
  agents: unknown[];
  checkpointCount: number;
  checkpointCoverage: number;
  totalBytes: number;
}) {
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const connectedAgents = agents.length;

  return (
    <>
      <PageStack>
        <StatGrid>
          <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
            <div>
              <StatLabel>Workspaces</StatLabel>
              <StatValue>{workspaces.length}</StatValue>
            </div>
            <StatDetail>
              {workspaces.length} workspace{workspaces.length === 1 ? "" : "s"} registered across{" "}
              {databases.length} database{databases.length === 1 ? "" : "s"}.
            </StatDetail>
          </ClickableStatCard>
          <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
            <div>
              <StatLabel>Stored Data</StatLabel>
              <StatValue>{formatBytes(totalBytes)}</StatValue>
            </div>
            <StatDetail>Total durable content tracked across all workspaces.</StatDetail>
          </ClickableStatCard>
          <ClickableStatCard onClick={() => navigate({ to: "/workspaces" })}>
            <div>
              <StatLabel>Checkpoints</StatLabel>
              <StatValue>{checkpointCount}</StatValue>
            </div>
            <StatDetail>{checkpointCoverage}% of workspaces have checkpoint history.</StatDetail>
          </ClickableStatCard>
          <ClickableStatCard onClick={() => navigate({ to: "/agents" })}>
            <div>
              <StatLabel>Connected Agents</StatLabel>
              <StatValue>{connectedAgents}</StatValue>
            </div>
            <StatDetail>
              {connectedAgents === 0
                ? "No agents are currently connected."
                : `${connectedAgents} live ${connectedAgents === 1 ? "agent" : "agents"} reporting workspace sessions.`}
            </StatDetail>
          </ClickableStatCard>
        </StatGrid>
        <LiveTopologyCard agents={agents as any} workspaces={workspaces as any} />
        <TemplateOnboardingBanner
          onBrowseTemplates={() => void navigate({ to: "/templates" })}
          onCreateBlank={() => setCreateOpen(true)}
        />
      </PageStack>
      <CreateWorkspaceDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
      />
    </>
  );
}

function TemplateOnboardingBanner({
  onBrowseTemplates,
  onCreateBlank,
}: {
  onBrowseTemplates: () => void;
  onCreateBlank: () => void;
}) {
  return (
    <TemplateBanner>
      <TemplateBannerBody>
        <TemplateBannerEyebrow>Next workspace</TemplateBannerEyebrow>
        <TemplateBannerTitle>Get started creating your next workspace</TemplateBannerTitle>
        <TemplateBannerText>
          Keep your starter workspace as the sandbox, then create a focused
          workspace from a template when you know what agents should share.
        </TemplateBannerText>
        <TemplateBannerActions>
          <Button size="large" onClick={onBrowseTemplates}>
            Browse templates
          </Button>
          <Button size="large" variant="secondary-fill" onClick={onCreateBlank}>
            Create blank workspace
          </Button>
        </TemplateBannerActions>
      </TemplateBannerBody>
      <TemplatePreviewList aria-label="Featured templates">
        {FEATURED_TEMPLATES.map((template) => (
          <TemplatePreviewItem key={template.id} $accent={template.accent}>
            <TemplatePreviewIcon $accent={template.accent}>
              <template.icon size="M" />
            </TemplatePreviewIcon>
            <TemplatePreviewCopy>
              <TemplatePreviewTitle>{template.title}</TemplatePreviewTitle>
              <TemplatePreviewMeta>{template.profileLabel}</TemplatePreviewMeta>
            </TemplatePreviewCopy>
          </TemplatePreviewItem>
        ))}
      </TemplatePreviewList>
    </TemplateBanner>
  );
}

function GettingStartedView({
  hasDatabase,
  onQuickstartCreated,
}: {
  hasDatabase: boolean;
  onQuickstartCreated: (workspace: AFSWorkspaceDetail) => void;
}) {
  const quickstartMutation = useQuickstartMutation();
  const quickstartErrorMessage = quickstartMutation.isError
    ? quickstartMutation.error.message || "Something went wrong."
    : null;

  const handleQuickstart = async () => {
    try {
      const result = await quickstartMutation.mutateAsync({});
      onQuickstartCreated(result.workspace);
    } catch {
      // Error is stored in quickstartMutation.error
    }
  };

  return (
    <PageStack>
      <HeroLayout>
        <HeroEyebrow>Agent Filesystem</HeroEyebrow>
        <HeroAnimationWrap>
          <AgentHeroAnimation />
        </HeroAnimationWrap>
        <Headline>
          A filesystem your AI agents can trust.
        </Headline>
        <Description>
          Give every agent a persistent, checkpointed workspace backed by
          Redis. Edit files, snapshot state, and replay history &mdash; all
          from one place.
        </Description>

        <CTABlock>
          <PrimaryCTA
            size="large"
            onClick={handleQuickstart}
            disabled={quickstartMutation.isPending}
          >
            {quickstartMutation.isPending
              ? "Setting up\u2026"
              : "Create my first workspace \u2192"}
          </PrimaryCTA>
          <CTAHint>
            {hasDatabase
              ? "We'll preload sample files so you can explore in seconds."
              : "Requires Redis running on localhost:6379"}
          </CTAHint>
          {quickstartErrorMessage ? (
            <QuickstartError>
              {quickstartErrorMessage.includes("cannot connect")
                ? "Could not connect to Redis at localhost:6379. Start Redis locally or add a remote database instead."
                : quickstartErrorMessage}
            </QuickstartError>
          ) : null}
        </CTABlock>

        <BenefitsGrid>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <ellipse cx="12" cy="5" rx="9" ry="3" />
                <path d="M3 5v14a9 3 0 0 0 18 0V5" />
                <path d="M3 12a9 3 0 0 0 18 0" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Persistent by default</BenefitTitle>
            <BenefitDesc>
              Workspaces live in Redis &mdash; no local state to sync,
              restore, or lose when you switch machines.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 12a9 9 0 1 0 9-9" />
                <polyline points="3 4 3 12 11 12" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>Checkpoint &amp; rollback</BenefitTitle>
            <BenefitDesc>
              Snapshot before risky changes. Restore the workspace to any
              previous state in seconds when an agent goes off the rails.
            </BenefitDesc>
          </Benefit>
          <Benefit>
            <BenefitIcon>
              <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="16 18 22 12 16 6" />
                <polyline points="8 6 2 12 8 18" />
              </svg>
            </BenefitIcon>
            <BenefitTitle>CLI &amp; MCP ready</BenefitTitle>
            <BenefitDesc>
              Mount workspaces locally with one command, or plug them into
              any MCP-capable agent &mdash; Claude, Cursor, Windsurf.
            </BenefitDesc>
          </Benefit>
        </BenefitsGrid>

        <FooterLink as={Link} to="/agent-guide">
          Read the full Agent Guide &rarr;
        </FooterLink>
      </HeroLayout>
    </PageStack>
  );
}

/* ── Styled components ── */

const HeroLayout = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: 24px 0 32px;
  max-width: 880px;
  margin: 0 auto;
`;

const HeroEyebrow = styled.div`
  color: var(--afs-accent);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const HeroAnimationWrap = styled.div`
  margin: 12px 0 8px;
  width: 100%;
  display: flex;
  justify-content: center;
`;

const Headline = styled.h2`
  margin: 8px 0 12px;
  color: var(--afs-ink);
  font-size: 42px;
  font-weight: 700;
  line-height: 1.1;
  letter-spacing: 0;
  max-width: 18ch;

  @media (max-width: 720px) {
    font-size: 32px;
  }
`;

const Description = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 17px;
  line-height: 1.55;
  max-width: 56ch;
`;

const CTABlock = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  margin: 28px 0 8px;
  width: 100%;
`;

const PrimaryCTA = styled(Button)`
  && {
    padding-left: 28px;
    padding-right: 28px;
    font-size: 15px;
    box-shadow: 0 10px 28px color-mix(in srgb, var(--afs-accent) 30%, transparent);
  }
`;

const CTAHint = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
`;

const QuickstartError = styled.div`
  color: #dc2626;
  font-size: 13px;
  line-height: 1.5;
  padding: 10px 14px;
  background: #fef2f2;
  border-radius: 10px;
  max-width: 480px;
`;

const BenefitsGrid = styled.div`
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  width: 100%;
  margin-top: 40px;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const Benefit = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  text-align: left;
  gap: 10px;
  padding: 22px 22px 24px;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel);
  transition: border-color 180ms ease, transform 180ms ease;

  &:hover {
    border-color: color-mix(in srgb, var(--afs-accent, #2563eb) 30%, var(--afs-line));
    transform: translateY(-2px);
  }
`;

const BenefitIcon = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: var(--afs-accent-soft, color-mix(in srgb, var(--afs-accent, #2563eb) 12%, transparent));
  color: var(--afs-accent, #2563eb);
`;

const BenefitTitle = styled.div`
  color: var(--afs-ink);
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.01em;
`;

const BenefitDesc = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13.5px;
  line-height: 1.6;
`;

const FooterLink = styled.a`
  margin-top: 32px;
  color: var(--afs-accent, #2563eb);
  font-size: 14px;
  font-weight: 600;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;

const TemplateBanner = styled.section`
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(320px, 0.9fr);
  gap: 24px;
  align-items: center;
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.94), rgba(248, 250, 252, 0.88)),
    color-mix(in srgb, var(--afs-accent, #2563eb) 7%, var(--afs-panel-strong));
  padding: 24px;

  @media (max-width: 980px) {
    grid-template-columns: 1fr;
  }

  @media (max-width: 640px) {
    padding: 18px;
  }
`;

const TemplateBannerBody = styled.div`
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
`;

const TemplateBannerEyebrow = styled.div`
  color: var(--afs-accent, #2563eb);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const TemplateBannerTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink);
  font-size: 24px;
  font-weight: 750;
  line-height: 1.2;
  letter-spacing: 0;
`;

const TemplateBannerText = styled.p`
  margin: 0;
  max-width: 62ch;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.6;
`;

const TemplateBannerActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 8px;
`;

const TemplatePreviewList = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;

  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
`;

const TemplatePreviewItem = styled.div<{ $accent: string }>`
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  padding: 12px;
  border: 1px solid color-mix(in srgb, ${({ $accent }) => $accent} 18%, var(--afs-line));
  border-radius: 12px;
  background: color-mix(in srgb, ${({ $accent }) => $accent} 7%, var(--afs-panel-strong));
`;

const TemplatePreviewIcon = styled.div<{ $accent: string }>`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: color-mix(in srgb, ${({ $accent }) => $accent} 16%, transparent);
  color: ${({ $accent }) => $accent};
`;

const TemplatePreviewCopy = styled.div`
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
`;

const TemplatePreviewTitle = styled.span`
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 750;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const TemplatePreviewMeta = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.3;
`;

const ClickableStatCardWrap = styled.div`
  height: 100%;
  cursor: pointer;
  transition: border-color 180ms ease, transform 180ms ease, box-shadow 180ms ease;
  border-radius: 16px;

  &:hover {
    transform: translateY(-2px);
    box-shadow: 0 6px 20px rgba(8, 6, 13, 0.08);
  }

  &:hover > * {
    border-color: var(--afs-accent, #2563eb);
  }

  > * {
    height: 100%;
  }
`;

function ClickableStatCard({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <ClickableStatCardWrap onClick={onClick}>
      <StatCard>{children}</StatCard>
    </ClickableStatCardWrap>
  );
}
