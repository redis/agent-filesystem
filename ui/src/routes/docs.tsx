import { createFileRoute, Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import styled from "styled-components";
import {
  DocPage,
  DocHero,
  DocHeroTitle,
  DocHeroSub,
  DocSection,
  DocHeading,
  DocSubheading,
  DocProse,
  InlineCode,
  CalloutBox,
  CrossLinkCard,
  CrossLinkText,
  CrossLinkTitle,
  CrossLinkDesc,
  CrossLinkArrow,
} from "../components/doc-kit";

export const Route = createFileRoute("/docs")({
  component: DocsPage,
});

function DocsPage() {
  return (
    <DocPage>
      <DocHero>
        <Eyebrow>CLI first</Eyebrow>
        <DocHeroTitle>Start with the AFS CLI</DocHeroTitle>
        <DocHeroSub>
          The fastest way to understand AFS is to log in, create a workspace,
          and mount it as a normal directory. Everything else builds from those
          commands.
        </DocHeroSub>
      </DocHero>

      <TerminalWindow title="Terminal">
        <TerminalComment>// authenticate the CLI to the cloud/control plane</TerminalComment>
        <TerminalCommand>afs login</TerminalCommand>
        <TerminalSpacer />
        <TerminalComment>// create a new workspace</TerminalComment>
        <TerminalCommand>afs workspace create myworkspace</TerminalCommand>
        <TerminalSpacer />
        <TerminalComment>// mount the workspace at ~/afs</TerminalComment>
        <TerminalCommand>afs up myworkspace ~/afs</TerminalCommand>
      </TerminalWindow>

      <CalloutBox $tone="tip">
        <DocProse>
          That is the core loop: authenticate once, create a workspace, then run
          <InlineCode>afs up</InlineCode> to make the workspace available on
          disk. Put files in <InlineCode>~/afs</InlineCode> and AFS keeps the
          workspace state backed by Redis.
        </DocProse>
      </CalloutBox>

      <DocSection>
        <DocHeading>What Those Commands Do</DocHeading>
        <StepList>
          <StepRow>
            <StepIndex>01</StepIndex>
            <StepBody>
              <StepTitle>Sign in</StepTitle>
              <DocProse>
                <InlineCode>afs login</InlineCode> connects your local CLI to
                AFS Cloud or a control plane. The CLI keeps the token locally so
                future commands can create and mount workspaces without another
                browser step.
              </DocProse>
            </StepBody>
          </StepRow>
          <StepRow>
            <StepIndex>02</StepIndex>
            <StepBody>
              <StepTitle>Create a workspace</StepTitle>
              <DocProse>
                <InlineCode>afs workspace create myworkspace</InlineCode> creates
                an empty workspace with an initial checkpoint. This workspace is
                the shared state agents and tools will work against.
              </DocProse>
            </StepBody>
          </StepRow>
          <StepRow>
            <StepIndex>03</StepIndex>
            <StepBody>
              <StepTitle>Mount it locally</StepTitle>
              <DocProse>
                <InlineCode>afs up myworkspace ~/afs</InlineCode> starts the
                local AFS runtime and exposes the workspace at{" "}
                <InlineCode>~/afs</InlineCode>. Use your editor, shell, and
                agents there like any other directory.
              </DocProse>
            </StepBody>
          </StepRow>
        </StepList>
      </DocSection>

      <DocSection>
        <DocHeading>Other Simple CLI Starts</DocHeading>
        <DocProse>
          Pick the path that matches what you already have. Each one starts from
          the terminal and keeps the workflow centered on the CLI.
        </DocProse>

        <ExampleList>
          <CliExample
            title="Start from existing code"
            description="Import a local directory into a new AFS workspace, then keep working in that same directory."
            commands={`afs login
afs workspace import myworkspace ~/src/my-app
afs up myworkspace ~/src/my-app`}
          />
          <CliExample
            title="Use the default mount path"
            description="Let AFS choose the mount path for the workspace, then reuse that selection on future runs."
            commands={`afs login
afs workspace create myworkspace
afs up myworkspace`}
          />
          <CliExample
            title="Checkpoint before a risky change"
            description="Save the current live workspace state before a refactor, migration, or agent handoff."
            commands={`afs checkpoint create myworkspace before-refactor
afs checkpoint list myworkspace`}
          />
          <CliExample
            title="Connect an agent over MCP"
            description="Launch the stdio MCP server for a workspace so an agent can read and write through AFS tools."
            commands={`afs login
afs mcp --workspace myworkspace`}
          />
        </ExampleList>
      </DocSection>

      <DocSection>
        <DocHeading>Daily CLI Commands</DocHeading>
        <CommandGrid>
          <CommandItem>
            <InlineCode>afs status</InlineCode>
            <span>Check the current runtime and workspace selection.</span>
          </CommandItem>
          <CommandItem>
            <InlineCode>afs down</InlineCode>
            <span>Stop the local runtime when you are done.</span>
          </CommandItem>
          <CommandItem>
            <InlineCode>afs workspace list</InlineCode>
            <span>See every workspace available to this CLI.</span>
          </CommandItem>
          <CommandItem>
            <InlineCode>afs workspace use myworkspace</InlineCode>
            <span>Set the default workspace for commands that omit one.</span>
          </CommandItem>
          <CommandItem>
            <InlineCode>afs checkpoint restore myworkspace initial</InlineCode>
            <span>Roll a workspace back to a known checkpoint.</span>
          </CommandItem>
          <CommandItem>
            <InlineCode>afs grep TODO</InlineCode>
            <span>Search workspace files directly through AFS.</span>
          </CommandItem>
        </CommandGrid>
      </DocSection>

      <DocSection>
        <DocHeading>Need the CLI?</DocHeading>
        <DocProse>
          Download the pre-built binary, run <InlineCode>afs login</InlineCode>,
          and come back to the terminal quickstart above.
        </DocProse>
        <DocSubheading>Install path</DocSubheading>
        <DocProse>
          The CLI is the primary workflow. The web UI is useful for browsing,
          activity, and management, but getting started should begin with{" "}
          <InlineCode>afs</InlineCode> in your shell.
        </DocProse>
        <InlineLink to="/downloads">Download AFS CLI</InlineLink>
      </DocSection>

      <CrossLinkCard as={Link} to="/agent-guide">
        <CrossLinkText>
          <CrossLinkTitle>Setting up an AI agent?</CrossLinkTitle>
          <CrossLinkDesc>
            Use the Agent Guide after the CLI is working to configure MCP tools
            and agent-specific workflows.
          </CrossLinkDesc>
        </CrossLinkText>
        <CrossLinkArrow>&rarr;</CrossLinkArrow>
      </CrossLinkCard>
    </DocPage>
  );
}

function CliExample(props: {
  title: string;
  description: string;
  commands: string;
}) {
  const promptedCommands = props.commands
    .split("\n")
    .map((command) => `> ${command}`)
    .join("\n");

  return (
    <ExampleRow>
      <ExampleCopy>
        <ExampleTitle>{props.title}</ExampleTitle>
        <DocProse>{props.description}</DocProse>
      </ExampleCopy>
      <MiniTerminal>{promptedCommands}</MiniTerminal>
    </ExampleRow>
  );
}

function TerminalWindow(props: { title: string; children: ReactNode }) {
  return (
    <TerminalFrame>
      <TerminalTopBar>
        <TerminalDots aria-hidden="true">
          <span />
          <span />
          <span />
        </TerminalDots>
        <TerminalTitle>{props.title}</TerminalTitle>
      </TerminalTopBar>
      <TerminalBody>{props.children}</TerminalBody>
    </TerminalFrame>
  );
}

function TerminalCommand({ children }: { children: ReactNode }) {
  return (
    <TerminalLine>
      <Prompt>&gt;</Prompt>
      <CommandText>{children}</CommandText>
    </TerminalLine>
  );
}

function TerminalComment({ children }: { children: ReactNode }) {
  return <TerminalLine $muted>{children}</TerminalLine>;
}

function TerminalSpacer() {
  return <TerminalGap aria-hidden="true" />;
}

const Eyebrow = styled.div`
  margin-bottom: 8px;
  color: var(--afs-accent, #064ea2);
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
`;

const TerminalFrame = styled.div`
  overflow: hidden;
  border: 1px solid var(--afs-line, #e6e6e6);
  border-radius: 8px;
  background: #101820;
  box-shadow: var(--afs-shadow, none);
`;

const TerminalTopBar = styled.div`
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 14px;
  min-height: 38px;
  padding: 0 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #1a242e;
`;

const TerminalDots = styled.div`
  display: flex;
  gap: 6px;

  span {
    width: 10px;
    height: 10px;
    border-radius: 999px;
    background: #6d6e71;
  }

  span:nth-child(1) {
    background: #dc2626;
  }

  span:nth-child(2) {
    background: #f59e0b;
  }

  span:nth-child(3) {
    background: #16a34a;
  }
`;

const TerminalTitle = styled.div`
  color: #d1d3d4;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12px;
`;

const TerminalBody = styled.div`
  padding: 22px;
  color: #f8f8f8;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 14px;
  line-height: 1.7;

  @media (max-width: 640px) {
    padding: 18px;
    font-size: 12px;
  }
`;

const TerminalLine = styled.div<{ $muted?: boolean }>`
  display: flex;
  min-height: 24px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  color: ${({ $muted }) => ($muted ? "#a7a9ac" : "#ffffff")};
`;

const Prompt = styled.span`
  flex: 0 0 auto;
  width: 24px;
  color: #16a34a;
`;

const CommandText = styled.span`
  color: #ffffff;
`;

const TerminalGap = styled.div`
  height: 12px;
`;

const StepList = styled.div`
  display: grid;
  gap: 18px;
  margin-top: 18px;
`;

const StepRow = styled.div`
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr);
  gap: 14px;
`;

const StepIndex = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: var(--afs-accent-soft, rgba(6, 78, 162, 0.1));
  color: var(--afs-accent, #064ea2);
  font-size: 12px;
  font-weight: 800;
`;

const StepBody = styled.div`
  min-width: 0;
`;

const StepTitle = styled.div`
  margin-bottom: 4px;
  color: var(--afs-ink, #282828);
  font-size: 14px;
  font-weight: 800;
`;

const ExampleList = styled.div`
  display: grid;
  gap: 22px;
  margin-top: 20px;
`;

const ExampleRow = styled.div`
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr);
  gap: 18px;
  align-items: start;
  padding-top: 22px;
  border-top: 1px solid var(--afs-line, #e6e6e6);

  &:first-child {
    padding-top: 0;
    border-top: none;
  }

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const ExampleCopy = styled.div`
  min-width: 0;
`;

const ExampleTitle = styled.div`
  margin-bottom: 4px;
  color: var(--afs-ink, #282828);
  font-size: 14px;
  font-weight: 800;
`;

const MiniTerminal = styled.pre`
  margin: 0;
  padding: 14px 16px;
  overflow-x: auto;
  border: 1px solid #1f2937;
  border-radius: 8px;
  background: #050807;
  color: #22c55e;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12.5px;
  line-height: 1.65;
  white-space: pre-wrap;
`;

const CommandGrid = styled.div`
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;

  @media (max-width: 760px) {
    grid-template-columns: 1fr;
  }
`;

const CommandItem = styled.div`
  display: grid;
  gap: 7px;
  padding: 12px 0;
  border-top: 1px solid var(--afs-line, #e6e6e6);
  color: var(--afs-muted, #6d6e71);
  font-size: 13px;
  line-height: 1.5;
`;

const InlineLink = styled(Link)`
  display: inline-flex;
  width: fit-content;
  margin-top: 16px;
  color: var(--afs-accent, #064ea2);
  font-size: 14px;
  font-weight: 800;
  text-decoration: none;

  &:hover {
    text-decoration: underline;
  }
`;
