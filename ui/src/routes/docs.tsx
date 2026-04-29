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
import { DocsTopicLinks } from "../features/docs/docs-topics";
import { pythonSdkSample, typescriptSdkSample } from "../features/docs/afs-samples";
import { searchBenchmark } from "../foundation/performance-data";

export const Route = createFileRoute("/docs")({
  component: DocsPage,
});

function DocsPage() {
  return (
    <DocPage>
      <DocHero>
        <Eyebrow>One-page primer</Eyebrow>
        <DocHeroTitle>What is AFS and How Does It Work?</DocHeroTitle>
        <DocHeroSub>
          AFS gives agents a filesystem-shaped workspace backed by Redis, with
          SDKs and the CLI as the fastest ways to start.
        </DocHeroSub>
      </DocHero>

      <PrimerPanel>
        <DocProse>
          Agent Filesystem (AFS) is a workspace system for AI agents. It gives
          agents a normal directory-shaped way to read files, write files,
          search trees, run tools, and share project state without being trapped
          on one machine's local disk.
        </DocProse>
        <DocProse>
          AFS works by keeping Redis as the canonical store for workspace
          metadata, manifests, blobs, live roots, checkpoints, and activity.
          The CLI, web UI, local sync or mount runtime, and MCP agent tools all
          operate on that same workspace model. Edits update live state;
          checkpoints are explicit saved moments you can restore or fork from.
        </DocProse>
      </PrimerPanel>

      <DocSection>
        <DocHeading>Start With The SDKs</DocHeading>
        <DocProse>
          Use the SDKs when an agent or app should create workspaces, mount
          them in process, read and write files, and run shell commands without
          asking the user to manage a local mount first.
        </DocProse>
        <DocProse>
          Install with <InlineCode>npm install @redis/afs-sdk</InlineCode> for
          TypeScript or <InlineCode>pip install redis-afs-sdk</InlineCode> for
          Python.
        </DocProse>
        <ExampleList>
          <SdkExample
            title="TypeScript"
            description="Create a repo-backed workspace, write a file, and run a command through the SDK mount."
            code={typescriptSdkSample}
          />
          <SdkExample
            title="Python"
            description="The Python SDK mirrors the TypeScript shape with snake_case methods."
            code={pythonSdkSample}
          />
        </ExampleList>
      </DocSection>

      <DocSection>
        <DocHeading>Start With The AFS CLI</DocHeading>
        <DocProse>
          The fastest way to understand AFS is to log in, create a workspace,
          and expose it as a normal local directory. Everything else builds from
          this loop.
        </DocProse>

        <TerminalWindow title="Terminal">
          <TerminalComment>// authenticate the CLI to the cloud/control plane</TerminalComment>
          <TerminalCommand>afs login</TerminalCommand>
          <TerminalSpacer />
          <TerminalComment>// create a new workspace</TerminalComment>
          <TerminalCommand>afs workspace create myworkspace</TerminalCommand>
          <TerminalSpacer />
          <TerminalComment>// mount or sync the workspace at ~/afs</TerminalComment>
          <TerminalCommand>afs up myworkspace ~/afs</TerminalCommand>
        </TerminalWindow>

        <CalloutBox $tone="tip">
          <DocProse>
            That is the core loop: authenticate once, create a workspace, then
            run <InlineCode>afs up</InlineCode> to make the workspace available
            on disk. Put files in <InlineCode>~/afs</InlineCode> and AFS keeps
            the workspace state backed by Redis.
          </DocProse>
        </CalloutBox>
      </DocSection>

      <DocSection>
        <DocHeading>The Whole Loop</DocHeading>
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
              <StepTitle>Expose it locally</StepTitle>
              <DocProse>
                <InlineCode>afs up myworkspace ~/afs</InlineCode> starts the
                local AFS runtime and exposes the workspace at{" "}
                <InlineCode>~/afs</InlineCode>. Use your editor, shell, and
                agents there like any other directory.
              </DocProse>
            </StepBody>
          </StepRow>
          <StepRow>
            <StepIndex>04</StepIndex>
            <StepBody>
              <StepTitle>Checkpoint the good state</StepTitle>
              <DocProse>
                <InlineCode>afs checkpoint create myworkspace before-refactor</InlineCode>{" "}
                saves a named restore point. Live edits are immediate, while
                checkpoints are deliberate moments in the workspace timeline.
              </DocProse>
            </StepBody>
          </StepRow>
        </StepList>
      </DocSection>

      <DocSection>
        <DocHeading>Docs Map</DocHeading>
        <DocProse>
          Use this page as the one-page primer, then jump into the detailed
          guides for each part of AFS.
        </DocProse>
        <DocsTopicLinks />
      </DocSection>

      <DocSection>
        <DocHeading>Common Questions</DocHeading>
        <FAQList>
          <FAQItem>
            <FAQQuestion>
              My data lives in GitHub, GitLab, S3, or Drive. Can I sync it to AFS?
            </FAQQuestion>
            <DocProse>
              Today, the clean path is to bring that data into a local
              directory first, then import or sync it with AFS. For Git
              upstreams, clone or check out the repo the way you already do,
              then run <InlineCode>afs workspace import</InlineCode> or{" "}
              <InlineCode>afs up --mode sync</InlineCode>. For non-Git systems
              like S3 or Google Drive, use their API or CLI in a small script
              and let AFS own the workspace state after the files land locally.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>How does egress metering work?</FAQQuestion>
            <DocProse>
              Local and Self-managed AFS do not add an AFS egress meter. Reads
              happen through sync, live mount, MCP tools, the CLI, or the
              control-plane API against your configured Redis/control-plane
              deployment. If you run AFS on hosted infrastructure, the normal
              provider bandwidth and Redis network policies still apply.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>
              Does AFS handle large files like datasets, models, and media?
            </FAQQuestion>
            <DocProse>
              Yes, within the shape of an agent workspace. AFS stores file
              content in Redis-backed external content keys, supports
              byte-range reads and writes in the mount path, and syncs changed
              large files in chunks. The default sync per-file cap is 2 GB, so
              keep generated artifacts and temporary bulk data out with{" "}
              <InlineCode>.afsignore</InlineCode> when they do not belong in
              the workspace timeline.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Does my agent really need versioning?</FAQQuestion>
            <DocProse>
              If the agent is doing throwaway scratch work, maybe not. If the
              work needs human review, rollback, audit history, or parallel
              exploration, versioning becomes the safety rail. In AFS, edits
              update the live workspace, checkpoints save deliberate restore
              points, and forks let another agent explore a second line of work
              without clobbering the first one.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Why can't I just use GitHub or GitLab?</FAQQuestion>
            <DocProse>
              You can, and you should keep using Git providers for source
              control, pull requests, and long-lived project history. AFS is
              for the live workspace around that source: generated files,
              prompts, logs, datasets, agent scratch state, checkpoints, forks,
              local sync, live mount, and MCP file tools. It gives agents a
              filesystem-shaped place to work before everything is ready to
              become a Git commit.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>
              How is this different from S3 or S3-style filesystems?
            </FAQQuestion>
            <DocProse>
              Object storage is excellent for durable blobs. AFS is a workspace
              system: it keeps a file tree, live edits, search, checkpoints,
              forks, and local execution surfaces together. Use S3-style
              storage for large durable objects and archival data; use AFS when
              agents need to edit a working tree, checkpoint the state, fork
              into parallel attempts, and keep normal tools pointed at a local
              path.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Is AFS POSIX compatible?</FAQQuestion>
            <DocProse>
              AFS is designed to feel like a normal filesystem to editors,
              shells, agents, and sandboxes. Sync mode gives you a real local
              directory on disk. Live mount mode exposes the workspace through
              NFS on macOS and FUSE on Linux. That covers the everyday Unix tool
              workflow, but AFS does not pretend to be a perfect replacement
              for a mature shared POSIX filesystem in every multi-writer edge
              case.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>How fast is AFS?</FAQQuestion>
            <DocProse>
              For the current Redis Search benchmark, indexed literal{" "}
              <InlineCode>afs grep</InlineCode> runs in tens of milliseconds:
              17.35 ms for a rare literal and 42.56 ms for a common literal on
              a 4,000-file markdown corpus. Sync mode keeps a real local
              directory on disk for editors and tools, while live mount and MCP
              give agents direct access to the same Redis-backed workspace.
            </DocProse>
          </FAQItem>
          <FAQItem>
            <FAQQuestion>Okay, but is AFS fast enough for agents?</FAQQuestion>
            <DocProse>
              AFS is built around time-to-useful-work. An agent can start from
              a workspace, search the tree, read the files it needs, write
              changes, and checkpoint the result without cloning or
              materializing everything up front. For regex-heavy searches, use{" "}
              <InlineCode>rg</InlineCode> on a synced or mounted workspace; for
              ordinary literal search, <InlineCode>afs grep</InlineCode> uses
              the indexed fast path when Redis Search is available.
            </DocProse>
          </FAQItem>
        </FAQList>
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
        <DocHeading>Search Performance</DocHeading>
        <DocProse>
          Simple literal <InlineCode>afs grep</InlineCode> uses the Redis Search
          index when it is available, then verifies candidate file contents
          through AFS. The latest local benchmark used{" "}
          {searchBenchmark.corpus} on {searchBenchmark.environment}.
        </DocProse>

        <PerformanceRows>
          {searchBenchmark.metrics.map((metric) => (
            <PerformanceRow key={metric.name}>
              <PerformanceName>{metric.name}</PerformanceName>
              <PerformanceValue>{metric.afs}</PerformanceValue>
              <PerformanceDetail>
                <span>BSD grep: {metric.grep}</span>
                <span>ripgrep: {metric.ripgrep}</span>
              </PerformanceDetail>
              <PerformanceSummary>{metric.summary}</PerformanceSummary>
            </PerformanceRow>
          ))}
        </PerformanceRows>

        <CalloutBox $tone="info">
          <DocProse>
            Literal searches are the indexed fast path. Regex searches are still
            honest about the work they do: they fall back to the advanced
            traversal path, so use <InlineCode>rg</InlineCode> on a mounted or
            synced workspace for regex-heavy scans.
          </DocProse>
        </CalloutBox>
        <InlineLink to="/docs/performance">Read the full performance notes</InlineLink>
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

function SdkExample(props: {
  title: string;
  description: string;
  code: string;
}) {
  return (
    <ExampleRow>
      <ExampleCopy>
        <ExampleTitle>{props.title}</ExampleTitle>
        <DocProse>{props.description}</DocProse>
      </ExampleCopy>
      <MiniTerminal>{props.code}</MiniTerminal>
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

const PrimerPanel = styled.div`
  display: grid;
  gap: 12px;
  padding: 24px 28px;
  border: 1px solid var(--afs-line, #e6e6e6);
  border-radius: 8px;
  background: var(--afs-panel-strong, #ffffff);

  @media (max-width: 720px) {
    padding: 20px;
  }
`;

const TerminalFrame = styled.div`
  overflow: hidden;
  margin-top: 18px;
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

const FAQList = styled.div`
  display: grid;
  margin-top: 18px;
  border-top: 1px solid var(--afs-line, #e6e6e6);
`;

const FAQItem = styled.div`
  display: grid;
  gap: 7px;
  padding: 16px 0;
  border-bottom: 1px solid var(--afs-line, #e6e6e6);
`;

const FAQQuestion = styled.div`
  color: var(--afs-ink, #282828);
  font-size: 14px;
  font-weight: 800;
  line-height: 1.4;
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

const PerformanceRows = styled.div`
  display: grid;
  margin-top: 18px;
  border-top: 1px solid var(--afs-line, #e6e6e6);
`;

const PerformanceRow = styled.div`
  display: grid;
  grid-template-columns: minmax(120px, 1fr) minmax(96px, auto) minmax(180px, 1.4fr) minmax(180px, 1.5fr);
  gap: 14px;
  align-items: center;
  padding: 14px 0;
  border-bottom: 1px solid var(--afs-line, #e6e6e6);

  @media (max-width: 780px) {
    grid-template-columns: 1fr;
    align-items: start;
    gap: 6px;
  }
`;

const PerformanceName = styled.div`
  color: var(--afs-ink, #282828);
  font-size: 14px;
  font-weight: 800;
`;

const PerformanceValue = styled.div`
  color: var(--afs-ink, #282828);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 20px;
  font-weight: 800;
  line-height: 1.1;
`;

const PerformanceDetail = styled.div`
  display: grid;
  gap: 3px;
  color: var(--afs-muted, #6d6e71);
  font-size: 12px;
  line-height: 1.35;
`;

const PerformanceSummary = styled.div`
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
