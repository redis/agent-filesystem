import { Link } from "@tanstack/react-router";
import {
  ArrowRight,
  BookOpen,
  Cloud,
  Code2,
  Database,
  GitBranch,
  LifeBuoy,
  LogIn,
  RotateCcw,
  Terminal,
  UserPlus,
} from "lucide-react";
import styled from "styled-components";

const githubUrl = "https://github.com/redis/agent-filesystem";

const featureCards = [
  {
    title: "One workspace model",
    icon: Database,
    body: "The CLI, SDKs, web UI, local sync or mount runtime, and MCP tools all work against the same live workspace.",
  },
  {
    title: "Checkpoint useful state",
    icon: RotateCcw,
    body: "File edits update the live workspace immediately. Checkpoints are explicit restore points when the state is worth saving.",
  },
  {
    title: "Fork parallel work",
    icon: GitBranch,
    body: "Fork a workspace when another agent should explore a second line of work without disturbing the main tree.",
  },
] as const;

const publicLinks = [
  {
    title: "Read the docs",
    body: "Start with the CLI-first primer and then go deeper on workspaces, MCP agents, SDKs, and deployment modes.",
    path: "/docs",
    icon: BookOpen,
  },
  {
    title: "Download the CLI",
    body: "Install the afs binary served by this control plane and connect a local directory to a workspace.",
    path: "/downloads",
    icon: Terminal,
  },
  {
    title: "Give this to an agent",
    body: "A compact operating guide for agents that need to create files, search, checkpoint, and restore.",
    path: "/agent-guide",
    icon: LifeBuoy,
  },
] as const;

export function PublicLandingPage() {
  return (
    <LandingPage>
      <HeroSection>
        <HeroKicker>
          <VersionPill>AFS Cloud</VersionPill>
          <span>backed by Redis</span>
          <span>open source</span>
          <span>MIT</span>
        </HeroKicker>

        <HeroTitle>
          a filesystem
          <HeroTitleLine>
            for <HeroStrike>humans</HeroStrike>{" "}
            <HeroAccent>agents</HeroAccent>.
          </HeroTitleLine>
        </HeroTitle>

        <HeroLead>
          AFS gives agents a filesystem-shaped workspace backed by Redis.
          Workspaces can be attached to real local directories, shared through
          MCP tools, checkpointed, restored, and forked without trapping agent
          state on one machine's local disk.
        </HeroLead>

        <HeroActions aria-label="Get started">
          <PrimaryCta to="/signup">
            <UserPlus size={18} strokeWidth={2} aria-hidden="true" />
            Create free account
            <ArrowRight size={18} strokeWidth={2} aria-hidden="true" />
          </PrimaryCta>
          <SecondaryCta to="/login">
            <LogIn size={18} strokeWidth={2} aria-hidden="true" />
            Log in to afs.cloud
          </SecondaryCta>
          <RepoCta href={githubUrl} target="_blank" rel="noreferrer">
            <GitBranch size={18} strokeWidth={2} aria-hidden="true" />
            Clone the repo
          </RepoCta>
        </HeroActions>

        <HeroFacts aria-label="Agent Filesystem facts">
          <HeroFact>
            <strong>3</strong>
            <span>ways in: CLI, MCP, SDKs</span>
          </HeroFact>
          <HeroFact>
            <strong>0</strong>
            <span>auto-checkpoint surprises</span>
          </HeroFact>
          <HeroFact>
            <strong>Redis</strong>
            <span>stores manifests, blobs, activity</span>
          </HeroFact>
        </HeroFacts>
      </HeroSection>

      <TerminalSection aria-label="AFS quick start commands">
        <TerminalHeader>
          <TerminalDots aria-hidden="true">
            <span />
            <span />
            <span />
          </TerminalDots>
          <TerminalTitle>~/code/my-repo - afs</TerminalTitle>
          <TerminalMeta>PID 0001 - zsh</TerminalMeta>
        </TerminalHeader>
        <TerminalBody>
          <TerminalComment># cloud start</TerminalComment>
          <TerminalLine><Prompt>$</Prompt> curl -fsSL https://afs.cloud/install.sh | bash</TerminalLine>
          <TerminalLine><Prompt>$</Prompt> afs auth login</TerminalLine>
          <TerminalLine><Prompt>$</Prompt> afs ws attach getting-started ~/getting-started</TerminalLine>
          <TerminalOutput><Ok>ok</Ok> attached getting-started at ~/getting-started</TerminalOutput>
          <TerminalGap />
          <TerminalComment># repo start</TerminalComment>
          <TerminalLine><Prompt>$</Prompt> git clone https://github.com/redis/agent-filesystem.git</TerminalLine>
          <TerminalLine><Prompt>$</Prompt> cd agent-filesystem &amp;&amp; make</TerminalLine>
          <TerminalLine><Prompt>$</Prompt> ./afs setup</TerminalLine>
          <TerminalOutput><Info>mode</Info> sync recommended - real directories, real tools</TerminalOutput>
        </TerminalBody>
      </TerminalSection>

      <SectionIntro>
        <SectionKicker>What It Is For</SectionKicker>
        <SectionTitle>Agent workspaces that behave like filesystems.</SectionTitle>
        <SectionText>
          Agent Filesystem gives agents a shared, versioned file tree. Each file
          tree is a workspace. Attach that workspace to a local directory so
          editors, shells, scripts, and agents can work through ordinary paths,
          while Redis keeps the durable source of truth.
        </SectionText>
      </SectionIntro>

      <FeatureGrid>
        {featureCards.map((feature) => {
          const Icon = feature.icon;
          return (
            <FeatureCard key={feature.title}>
              <FeatureIcon>
                <Icon size={20} strokeWidth={1.9} aria-hidden="true" />
              </FeatureIcon>
              <FeatureTitle>{feature.title}</FeatureTitle>
              <FeatureBody>{feature.body}</FeatureBody>
            </FeatureCard>
          );
        })}
      </FeatureGrid>

      <FlowSection>
        <FlowCopy>
          <SectionKicker>How It Feels</SectionKicker>
          <SectionTitle>Authenticate once. Attach a workspace. Checkpoint the good state.</SectionTitle>
          <SectionText>
            The core loop is small: sign in, attach a workspace, edit locally or
            through MCP, checkpoint useful state, and restore or fork when the
            work needs another line.
          </SectionText>
        </FlowCopy>
        <FlowDiagram aria-label="AFS flow">
          <FlowNode>
            <Cloud size={18} strokeWidth={1.9} aria-hidden="true" />
            <strong>AFS Cloud</strong>
            <span>browser auth and hosted UI</span>
          </FlowNode>
          <FlowRail />
          <FlowNode>
            <Terminal size={18} strokeWidth={1.9} aria-hidden="true" />
            <strong>Local directory</strong>
            <span>sync or mount mode</span>
          </FlowNode>
          <FlowRail />
          <FlowNode>
            <Code2 size={18} strokeWidth={1.9} aria-hidden="true" />
            <strong>MCP + SDKs</strong>
            <span>agent file, search, checkpoint tools</span>
          </FlowNode>
        </FlowDiagram>
      </FlowSection>

      <PublicLinks aria-label="Public resources">
        {publicLinks.map((item) => {
          const Icon = item.icon;
          return (
            <PublicLinkCard key={item.path} to={item.path}>
              <PublicLinkIcon>
                <Icon size={18} strokeWidth={1.9} aria-hidden="true" />
              </PublicLinkIcon>
              <PublicLinkText>
                <strong>{item.title}</strong>
                <span>{item.body}</span>
              </PublicLinkText>
              <ArrowRight size={18} strokeWidth={1.9} aria-hidden="true" />
            </PublicLinkCard>
          );
        })}
      </PublicLinks>

      <BottomCta>
        <BottomCtaText>
          <SectionKicker>Start Building</SectionKicker>
          <h2>Use the hosted product, or run it from source.</h2>
          <p>
            Create a free AFS Cloud account when you want browser auth and the
            hosted UI. Clone the repo when you want the open-source path and
            your own Redis-backed setup.
          </p>
        </BottomCtaText>
        <BottomCtaActions>
          <PrimaryCta to="/signup">
            <UserPlus size={18} strokeWidth={2} aria-hidden="true" />
            Create free account
          </PrimaryCta>
          <RepoCta href={githubUrl} target="_blank" rel="noreferrer">
            <GitBranch size={18} strokeWidth={2} aria-hidden="true" />
            Clone repo
          </RepoCta>
        </BottomCtaActions>
      </BottomCta>
    </LandingPage>
  );
}

const LandingPage = styled.div`
  width: min(100%, 1480px);
  margin: 0 auto;
  padding: 88px 32px 72px;

  @media (max-width: 860px) {
    padding: 56px 18px 48px;
  }
`;

const HeroSection = styled.section`
  max-width: 980px;
  min-height: calc(100vh - 270px);
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: 32px 0 56px;

  @media (max-width: 860px) {
    min-height: auto;
    padding: 16px 0 40px;
  }
`;

const HeroKicker = styled.div`
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 14px 22px;
  margin-bottom: 42px;
  color: var(--afs-ink-dim);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-md);
  letter-spacing: var(--afs-tr-caps);
  text-transform: lowercase;

  span:not(:first-child)::before {
    content: ".";
    margin-right: 22px;
    color: var(--afs-ink-faint);
  }

  @media (max-width: 640px) {
    gap: 10px;
    margin-bottom: 30px;

    span:not(:first-child)::before {
      content: none;
    }
  }
`;

const VersionPill = styled.span`
  display: inline-flex;
  align-items: center;
  min-height: 32px;
  border: 1px solid var(--afs-accent);
  padding: 5px 12px;
  color: var(--afs-accent);
  letter-spacing: 0.08em;
`;

const HeroTitle = styled.h1`
  margin: 0;
  color: var(--afs-ink);
  font-family: var(--afs-font-mono);
  font-size: 92px;
  font-weight: var(--afs-fw-medium);
  line-height: 0.98;
  letter-spacing: 0;

  @media (max-width: 980px) {
    font-size: 72px;
  }

  @media (max-width: 640px) {
    font-size: 46px;
    line-height: 1.05;
  }
`;

const HeroTitleLine = styled.span`
  display: block;
`;

const HeroStrike = styled.s`
  color: var(--afs-ink-faint);
  text-decoration-color: currentColor;
  text-decoration-thickness: 6px;
  text-decoration-skip-ink: none;

  @media (max-width: 640px) {
    text-decoration-thickness: 4px;
  }
`;

const HeroAccent = styled.span`
  color: var(--afs-accent);
`;

const HeroLead = styled.p`
  max-width: 760px;
  margin: 36px 0 0;
  color: var(--afs-ink-soft);
  font-family: var(--afs-font-mono);
  font-size: 20px;
  line-height: 1.65;
  letter-spacing: 0;

  @media (max-width: 640px) {
    margin-top: 24px;
    font-size: 16px;
  }
`;

const HeroActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin-top: 42px;
`;

const ctaBase = `
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 52px;
  border-radius: var(--afs-r-2);
  padding: 14px 22px;
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-md);
  font-weight: var(--afs-fw-semi);
  letter-spacing: 0;
  line-height: 1;
  text-decoration: none;
  transition: transform var(--afs-dur-fast) var(--afs-ease), border-color var(--afs-dur-fast) var(--afs-ease), background var(--afs-dur-fast) var(--afs-ease), color var(--afs-dur-fast) var(--afs-ease);

  &:hover {
    transform: translateY(-1px);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

const PrimaryCta = styled(Link)`
  ${ctaBase}
  border: 1px solid var(--afs-accent);
  background: var(--afs-accent);
  color: var(--afs-ink-on-accent);

  &:hover {
    border-color: var(--afs-accent);
    background: var(--afs-accent);
    color: var(--afs-ink-on-accent);
  }
`;

const SecondaryCta = styled(Link)`
  ${ctaBase}
  border: 1px solid var(--afs-line-strong);
  background: color-mix(in srgb, var(--afs-bg-1) 80%, transparent);
  color: var(--afs-ink);

  &:hover {
    border-color: var(--afs-accent);
    background: var(--afs-accent-soft);
    color: var(--afs-accent);
  }
`;

const RepoCta = styled.a`
  ${ctaBase}
  border: 1px solid var(--afs-line-strong);
  background: transparent;
  color: var(--afs-ink);

  &:hover {
    border-color: var(--afs-line-strong);
    background: var(--afs-bg-2);
    color: var(--afs-ink);
  }
`;

const HeroFacts = styled.div`
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  margin-top: 52px;
  padding-top: 24px;
  border-top: 1px dashed var(--afs-line-strong);

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const HeroFact = styled.div`
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
  color: var(--afs-ink-dim);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-sm);
  letter-spacing: var(--afs-tr-body);

  strong {
    color: var(--afs-ink);
    font-size: var(--afs-fz-lg);
    letter-spacing: 0;
  }
`;

const TerminalSection = styled.section`
  overflow: hidden;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  background: var(--afs-bg-1);
`;

const TerminalHeader = styled.div`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
  min-height: 48px;
  padding: 0 18px;
  border-bottom: 1px solid var(--afs-line-strong);
  background: var(--afs-bg-2);
  color: var(--afs-ink-dim);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-sm);

  @media (max-width: 640px) {
    grid-template-columns: auto minmax(0, 1fr);
  }
`;

const TerminalDots = styled.span`
  display: flex;
  gap: 7px;

  span {
    width: 10px;
    height: 10px;
    border-radius: var(--afs-r-pill);
    background: var(--afs-err);
  }

  span:nth-child(2) {
    background: var(--afs-amber);
  }

  span:nth-child(3) {
    background: var(--afs-accent);
  }
`;

const TerminalTitle = styled.span`
  min-width: 0;
  overflow: hidden;
  color: var(--afs-ink);
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

const TerminalMeta = styled.span`
  color: var(--afs-ink-dim);

  @media (max-width: 640px) {
    display: none;
  }
`;

const TerminalBody = styled.div`
  margin: 0;
  min-height: 330px;
  padding: 32px 36px;
  overflow-x: auto;
  color: var(--afs-ink);
  font-family: var(--afs-font-mono);
  font-size: 17px;
  line-height: 1.8;
  letter-spacing: 0;
  white-space: pre;

  @media (max-width: 760px) {
    min-height: 0;
    padding: 22px 18px;
    font-size: 13px;
  }
`;

const TerminalLine = styled.div`
  color: var(--afs-ink);
`;

const TerminalOutput = styled.div`
  color: var(--afs-ink-dim);
`;

const TerminalComment = styled.div`
  color: var(--afs-ink-dim);
`;

const TerminalGap = styled.div`
  height: 14px;
`;

const Prompt = styled.span`
  color: var(--afs-accent);
`;

const Ok = styled.span`
  color: var(--afs-ok);
`;

const Info = styled.span`
  color: var(--afs-info);
`;

const SectionIntro = styled.section`
  max-width: 840px;
  margin-top: 96px;

  @media (max-width: 760px) {
    margin-top: 64px;
  }
`;

const SectionKicker = styled.div`
  color: var(--afs-accent);
  font-family: var(--afs-font-mono);
  font-size: var(--afs-fz-xs);
  letter-spacing: var(--afs-tr-caps);
  text-transform: uppercase;
`;

const SectionTitle = styled.h2`
  margin: 10px 0 0;
  color: var(--afs-ink);
  font-family: var(--afs-font-mono);
  font-size: 34px;
  font-weight: var(--afs-fw-medium);
  line-height: 1.18;
  letter-spacing: 0;

  @media (max-width: 640px) {
    font-size: 26px;
  }
`;

const SectionText = styled.p`
  max-width: 760px;
  margin: 18px 0 0;
  color: var(--afs-ink-soft);
  font-family: var(--afs-font-mono);
  font-size: 16px;
  line-height: 1.7;
  letter-spacing: 0;
`;

const FeatureGrid = styled.section`
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  margin-top: 32px;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
  }
`;

const FeatureCard = styled.article`
  display: grid;
  gap: 12px;
  min-height: 210px;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  background: color-mix(in srgb, var(--afs-bg-1) 88%, transparent);
  padding: 22px;
`;

const FeatureIcon = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  color: var(--afs-accent);
  background: var(--afs-accent-soft);
`;

const FeatureTitle = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-family: var(--afs-font-mono);
  font-size: 18px;
  font-weight: var(--afs-fw-medium);
  line-height: 1.3;
  letter-spacing: 0;
`;

const FeatureBody = styled.p`
  margin: 0;
  color: var(--afs-ink-soft);
  font-family: var(--afs-font-mono);
  font-size: 14px;
  line-height: 1.65;
  letter-spacing: 0;
`;

const FlowSection = styled.section`
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(360px, 1.1fr);
  gap: 40px;
  align-items: center;
  margin-top: 96px;

  @media (max-width: 960px) {
    grid-template-columns: 1fr;
  }

  @media (max-width: 760px) {
    margin-top: 64px;
  }
`;

const FlowCopy = styled.div`
  min-width: 0;
`;

const FlowDiagram = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 1fr) 54px minmax(0, 1fr) 54px minmax(0, 1fr);
  align-items: stretch;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
    gap: 10px;
  }
`;

const FlowNode = styled.div`
  display: grid;
  align-content: start;
  gap: 8px;
  min-height: 150px;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  background: var(--afs-bg-1);
  padding: 18px;
  color: var(--afs-ink-soft);
  font-family: var(--afs-font-mono);
  font-size: 13px;
  line-height: 1.5;

  svg {
    color: var(--afs-accent);
  }

  strong {
    color: var(--afs-ink);
    font-size: 15px;
    font-weight: var(--afs-fw-medium);
    letter-spacing: 0;
  }

  span {
    color: var(--afs-ink-dim);
  }
`;

const FlowRail = styled.div`
  align-self: center;
  height: 1px;
  background: var(--afs-line-strong);

  @media (max-width: 760px) {
    width: 1px;
    height: 18px;
    justify-self: center;
  }
`;

const PublicLinks = styled.section`
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  margin-top: 96px;

  @media (max-width: 960px) {
    grid-template-columns: 1fr;
    margin-top: 64px;
  }
`;

const PublicLinkCard = styled(Link)`
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: 16px;
  align-items: center;
  min-height: 132px;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  background: var(--afs-bg-1);
  padding: 18px;
  color: var(--afs-ink);
  text-decoration: none;
  transition: transform var(--afs-dur-fast) var(--afs-ease), border-color var(--afs-dur-fast) var(--afs-ease), background var(--afs-dur-fast) var(--afs-ease);

  &:hover {
    border-color: var(--afs-accent);
    background: var(--afs-accent-soft);
    transform: translateY(-1px);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-focus);
    outline-offset: 2px;
  }
`;

const PublicLinkIcon = styled.div`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  border: 1px solid var(--afs-line-strong);
  border-radius: var(--afs-r-2);
  color: var(--afs-accent);
`;

const PublicLinkText = styled.span`
  display: grid;
  gap: 5px;
  min-width: 0;
  font-family: var(--afs-font-mono);

  strong {
    color: var(--afs-ink);
    font-size: 15px;
    font-weight: var(--afs-fw-medium);
    letter-spacing: 0;
  }

  span {
    color: var(--afs-ink-dim);
    font-size: 13px;
    line-height: 1.55;
    letter-spacing: 0;
  }
`;

const BottomCta = styled.section`
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 28px;
  align-items: end;
  margin-top: 96px;
  border-top: 1px dashed var(--afs-line-strong);
  padding-top: 36px;

  @media (max-width: 860px) {
    grid-template-columns: 1fr;
    margin-top: 64px;
  }
`;

const BottomCtaText = styled.div`
  min-width: 0;

  h2 {
    margin: 10px 0 0;
    color: var(--afs-ink);
    font-family: var(--afs-font-mono);
    font-size: 28px;
    font-weight: var(--afs-fw-medium);
    line-height: 1.2;
    letter-spacing: 0;
  }

  p {
    max-width: 720px;
    margin: 14px 0 0;
    color: var(--afs-ink-soft);
    font-family: var(--afs-font-mono);
    font-size: 15px;
    line-height: 1.65;
    letter-spacing: 0;
  }
`;

const BottomCtaActions = styled.div`
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: flex-end;

  @media (max-width: 860px) {
    justify-content: flex-start;
  }
`;
