import { Button } from "@redis-ui/components";
import JSZip from "jszip";
import { useState } from "react";
import styled from "styled-components";
import { getControlPlaneURL } from "../../foundation/api/afs";
import type {
  AFSMCPToken,
  AFSWorkspaceDetail,
} from "../../foundation/types/afs";
import {
  buildGenericAgentInstructions,
  buildSkillInstallCommand,
  templateMCPUrl,
  templateServerName,
  templateSkillName,
} from "./agent-install";
import {
  buildClaudeMcpAddCommand,
  buildClaudePlugin,
} from "./claude-plugin";
import {
  buildCodexSkillInstallCommand,
  buildCodexTomlConfig,
} from "./codex-install";
import { copyTextToClipboard } from "./clipboard";
import type { Template } from "./templates-data";

type Props = {
  workspace: AFSWorkspaceDetail;
  token: AFSMCPToken;
  template: Template;
  seededCount?: number;
  showFreshBanner?: boolean;
};

type SetupChoice = "claude-code" | "codex" | "manual";

export function buildHostedMCPConfig(
  workspaceName: string,
  controlPlaneUrl: string,
  token: string,
) {
  return JSON.stringify(
    {
      mcpServers: {
        [templateServerName(workspaceName)]: {
          url: templateMCPUrl(controlPlaneUrl),
          headers: {
            Authorization: `Bearer ${token || "<token-not-returned>"}`,
          },
        },
      },
    },
    null,
    2,
  );
}

export async function downloadClaudePlugin(args: {
  template: Template;
  workspaceName: string;
  controlPlaneUrl: string;
  token: string;
}): Promise<void> {
  const files = buildClaudePlugin(args);
  const zip = new JSZip();
  for (const file of files) {
    const isExecutable = file.path.endsWith(".sh");
    zip.file(file.path, file.content, {
      unixPermissions: isExecutable ? 0o755 : 0o644,
    });
  }
  const blob = await zip.generateAsync({ type: "blob", platform: "UNIX" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `${templateServerName(args.workspaceName)}.zip`;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  window.setTimeout(() => URL.revokeObjectURL(url), 2000);
}

export function TemplateInstallDetail({
  workspace,
  token,
  template,
  seededCount,
  showFreshBanner,
}: Props) {
  const [copied, setCopied] = useState<string | null>(null);
  const [copyError, setCopyError] = useState<string | null>(null);
  const hasAgentSkill = Boolean(template.agentSkill);
  const [choice, setChoice] = useState<SetupChoice>(
    hasAgentSkill ? "claude-code" : "manual",
  );

  function copy(text: string, label: string) {
    void copyTextToClipboard(text).then((ok) => {
      if (!ok) {
        setCopied(null);
        setCopyError(label);
        window.setTimeout(() => setCopyError(null), 2500);
        return;
      }

      setCopyError(null);
      setCopied(label);
      window.setTimeout(() => {
        setCopied((current) => (current === label ? null : current));
      }, 2000);
    });
  }

  function copyButtonText(label: string, defaultText: string) {
    if (copyError === label) return "Copy failed";
    if (copied === label) return "Copied!";
    return defaultText;
  }

  const tokenValue = token.token ?? "";
  const serverName = templateServerName(workspace.name);
  const skillName = templateSkillName(workspace.name);
  const mcpUrl = templateMCPUrl(getControlPlaneURL());
  const mcpConfig = buildHostedMCPConfig(
    workspace.name,
    getControlPlaneURL(),
    tokenValue,
  );
  const codexConfig = buildCodexTomlConfig(workspace.name, tokenValue);
  const claudeMcpAddCmd = buildClaudeMcpAddCommand({
    workspaceName: workspace.name,
    mcpUrl,
    token: tokenValue,
  });
  const skillInstallCmd = buildSkillInstallCommand({
    workspaceName: workspace.name,
    template,
    skillsRoot: "~/.claude/skills",
  });
  const codexSkillInstallCmd = buildCodexSkillInstallCommand({
    workspaceName: workspace.name,
    template,
  });
  const genericInstructions = buildGenericAgentInstructions({
    workspaceName: workspace.name,
    template,
  });

  return (
    <Panel>
      {showFreshBanner && seededCount != null ? (
        <SeededBanner>
          <SeededDot aria-hidden>&#10003;</SeededDot>
          <SeededText>
            Seeded <strong>{seededCount}</strong> file
            {seededCount === 1 ? "" : "s"} into{" "}
            <code>{workspace.name}</code>. The workspace layout is ready.
          </SeededText>
        </SeededBanner>
      ) : null}

      <ChoiceRow>
        <SectionLabel>Choose your setup</SectionLabel>
        <ChoiceStrip>
          <ChoiceCard
            type="button"
            $active={choice === "claude-code"}
            onClick={() => setChoice("claude-code")}
            aria-pressed={choice === "claude-code"}
          >
            <ChoiceTitle>Setup for Claude Code</ChoiceTitle>
            <ChoiceHint>
              MCP command plus an auto-trigger skill when this template has one.
            </ChoiceHint>
          </ChoiceCard>
          <ChoiceCard
            type="button"
            $active={choice === "codex"}
            onClick={() => setChoice("codex")}
            aria-pressed={choice === "codex"}
          >
            <ChoiceTitle>Setup for Codex</ChoiceTitle>
            <ChoiceHint>
              Config block plus a user-scope skill when this template has one.
            </ChoiceHint>
          </ChoiceCard>
          <ChoiceCard
            type="button"
            $active={choice === "manual"}
            onClick={() => setChoice("manual")}
            aria-pressed={choice === "manual"}
          >
            <ChoiceTitle>Manual</ChoiceTitle>
            <ChoiceHint>Raw MCP JSON for any compatible client.</ChoiceHint>
          </ChoiceCard>
        </ChoiceStrip>
      </ChoiceRow>

      {choice === "claude-code" ? (
        <Section>
          <Step>
            <StepHeader>
              <StepNumber>1</StepNumber>
              <StepTitle>Register the MCP server</StepTitle>
            </StepHeader>
            <SectionHint>
              Paste this into your terminal. It registers{" "}
              <code>{serverName}</code> at user scope with the bearer token
              pre-embedded.
            </SectionHint>
            <CodeBlock>{claudeMcpAddCmd}</CodeBlock>
            <InlineActionsRight>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => copy(claudeMcpAddCmd, "claude-mcp-add")}
              >
                {copyButtonText("claude-mcp-add", "Copy command")}
              </Button>
            </InlineActionsRight>
          </Step>

          {skillInstallCmd ? (
            <Step>
              <StepHeader>
                <StepNumber>2</StepNumber>
                <StepTitle>Install the auto-trigger skill (optional)</StepTitle>
              </StepHeader>
              <SectionHint>
                Writes <code>~/.claude/skills/{skillName}/SKILL.md</code>{" "}
                so Claude reaches for this workspace automatically when
                relevant. Skip if you prefer to call MCP tools explicitly.
              </SectionHint>
              <CodeBlock>{skillInstallCmd}</CodeBlock>
              <InlineActionsRight>
                <Button
                  size="small"
                  variant="secondary-fill"
                  onClick={() => copy(skillInstallCmd, "skill-install")}
                >
                  {copyButtonText("skill-install", "Copy command")}
                </Button>
              </InlineActionsRight>
            </Step>
          ) : null}

          <HintLine>
            Restart Claude Code after running the command(s). Type{" "}
            <code>/mcp</code> inside Claude Code to verify{" "}
            <code>{serverName}</code> is connected.
          </HintLine>

          {hasAgentSkill ? (
            <FallbackRow>
              <HintLine>
                Advanced: download a Claude Code plugin bundle with the same
                MCP server and skill pre-wired. The direct commands above are
                the recommended path.
              </HintLine>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => {
                  void downloadClaudePlugin({
                    template,
                    workspaceName: workspace.name,
                    controlPlaneUrl: getControlPlaneURL(),
                    token: tokenValue,
                  });
                }}
              >
                Download plugin (.zip)
              </Button>
            </FallbackRow>
          ) : null}
        </Section>
      ) : null}

      {choice === "codex" ? (
        <Section>
          <Step>
            <StepHeader>
              <StepNumber>1</StepNumber>
              <StepTitle>Register the MCP server</StepTitle>
            </StepHeader>
            <SectionHint>
              Append this block to <code>~/.codex/config.toml</code>. It uses
              Codex&apos;s documented <code>http_headers</code> format for
              streamable HTTP MCP servers.
            </SectionHint>
            <CodeBlock>{codexConfig}</CodeBlock>
            <InlineActionsRight>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => copy(codexConfig, "codex-toml")}
              >
                {copyButtonText("codex-toml", "Copy TOML")}
              </Button>
            </InlineActionsRight>
          </Step>

          {codexSkillInstallCmd ? (
            <Step>
              <StepHeader>
                <StepNumber>2</StepNumber>
                <StepTitle>Install the auto-trigger skill</StepTitle>
              </StepHeader>
              <SectionHint>
                Run this in your terminal. It writes{" "}
                <code>$HOME/.agents/skills/{skillName}/SKILL.md</code>,
                which is one of Codex&apos;s built-in skill discovery paths.
              </SectionHint>
              <CodeBlock>{codexSkillInstallCmd}</CodeBlock>
              <InlineActionsRight>
                <Button
                  size="small"
                  variant="secondary-fill"
                  onClick={() => copy(codexSkillInstallCmd, "codex-skill")}
                >
                  {copyButtonText("codex-skill", "Copy command")}
                </Button>
              </InlineActionsRight>
            </Step>
          ) : null}

          <HintLine>
            Restart Codex after adding the MCP block
            {codexSkillInstallCmd ? " and skill" : ""}. Launch Codex from any
            repo and ask about this workspace normally.
          </HintLine>
        </Section>
      ) : null}

      {choice === "manual" ? (
        <Section>
          <Step>
            <StepHeader>
              <StepNumber>1</StepNumber>
              <StepTitle>Register the MCP server</StepTitle>
            </StepHeader>
            <SectionHint>
              Paste this into your MCP client config (e.g.{" "}
              <code>.mcp.json</code> or <code>~/.claude.json</code> under{" "}
              <code>mcpServers</code>).
            </SectionHint>
            <CodeBlock>{mcpConfig}</CodeBlock>
            <InlineActionsRight>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => copy(mcpConfig, "config")}
              >
                {copyButtonText("config", "Copy MCP config")}
              </Button>
            </InlineActionsRight>
          </Step>

          <Step>
            <StepHeader>
              <StepNumber>2</StepNumber>
              <StepTitle>Give your agent the template instructions</StepTitle>
            </StepHeader>
            <SectionHint>
              Use this with clients that do not support a local skill folder.
              It ties the template behavior to <code>{serverName}</code>.
            </SectionHint>
            <CodeBlock>{genericInstructions}</CodeBlock>
            <InlineActionsRight>
              <Button
                size="small"
                variant="secondary-fill"
                onClick={() => copy(genericInstructions, "instructions")}
              >
                {copyButtonText("instructions", "Copy instructions")}
              </Button>
            </InlineActionsRight>
          </Step>

          <HintLine>
            <strong>Prefer the CLI?</strong> Run{" "}
            <code>
              afs mcp --workspace {workspace.name} --profile {template.profile}
            </code>{" "}
            after <code>afs login</code> with the token above.
          </HintLine>
        </Section>
      ) : null}

      <Section>
        <SectionLabel>Then ask your agent</SectionLabel>
        <FirstPrompt>&ldquo;{template.firstPrompt}&rdquo;</FirstPrompt>
      </Section>
    </Panel>
  );
}

const Panel = styled.div`
  display: grid;
  gap: 20px;
`;

const ChoiceRow = styled.div`
  display: grid;
  gap: 10px;
`;

const ChoiceStrip = styled.div`
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
  }
`;

const ChoiceCard = styled.button<{ $active?: boolean; $disabled?: boolean }>`
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px;
  border-radius: 14px;
  border: 1.5px solid
    ${(p) => (p.$active ? "var(--afs-accent, #2563eb)" : "var(--afs-line)")};
  background: ${(p) =>
    p.$active
      ? "color-mix(in srgb, var(--afs-accent, #2563eb) 8%, transparent)"
      : "var(--afs-panel)"};
  text-align: left;
  cursor: ${(p) => (p.$disabled ? "not-allowed" : "pointer")};
  opacity: ${(p) => (p.$disabled && !p.$active ? 0.55 : 1)};
  transition:
    border-color 120ms ease,
    background 120ms ease;

  &:hover:not(:disabled) {
    border-color: var(--afs-accent, #2563eb);
  }
`;

const ChoiceTitle = styled.span`
  color: var(--afs-ink);
  font-size: 14px;
  font-weight: 700;
`;

const ChoiceHint = styled.span`
  color: var(--afs-muted);
  font-size: 12px;
  line-height: 1.45;
`;

const Section = styled.div`
  display: grid;
  gap: 12px;
  padding: 14px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: var(--afs-panel);
`;

const Step = styled.div`
  display: grid;
  gap: 8px;
  padding-bottom: 12px;
  border-bottom: 1px dashed var(--afs-line);

  &:last-of-type {
    border-bottom: none;
    padding-bottom: 0;
  }
`;

const StepHeader = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
`;

const StepNumber = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--afs-accent, #2563eb);
  color: #fff;
  font-size: 12px;
  font-weight: 800;
`;

const StepTitle = styled.h5`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13.5px;
  font-weight: 700;
`;

const FallbackRow = styled.div`
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 6px;
  padding: 10px 12px;
  border: 1px dashed var(--afs-line);
  border-radius: 10px;
  background: color-mix(in srgb, var(--afs-line) 20%, transparent);

  @media (max-width: 600px) {
    flex-direction: column;
    align-items: stretch;
  }
`;

const SectionLabel = styled.h4`
  margin: 0;
  color: var(--afs-ink);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.02em;
`;

const SectionHint = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 11.5px;
    padding: 1px 6px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--afs-line) 60%, transparent);
    color: var(--afs-ink);
  }
`;

const SeededBanner = styled.div`
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  border-radius: 12px;
  background: color-mix(in srgb, #22c55e 14%, transparent);
  border: 1px solid color-mix(in srgb, #22c55e 40%, transparent);
`;

const SeededDot = styled.span`
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #16a34a;
  color: #fff;
  font-size: 13px;
  font-weight: 800;
`;

const SeededText = styled.span`
  color: var(--afs-ink);
  font-size: 13px;
  line-height: 1.5;

  strong {
    font-weight: 700;
  }

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 12px;
    padding: 1px 6px;
    border-radius: 4px;
    background: color-mix(in srgb, #22c55e 16%, transparent);
    color: var(--afs-ink);
  }
`;

const CodeBlock = styled.pre`
  margin: 0;
  padding: 14px 16px;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.94);
  color: #e2e8f0;
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
`;

const InlineActionsRight = styled.div`
  display: flex;
  justify-content: flex-end;
`;

const HintLine = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 12.5px;
  line-height: 1.55;

  code {
    font-family: "SF Mono", "Fira Code", "Consolas", monospace;
    font-size: 11.5px;
    padding: 1px 6px;
    margin: 0 2px;
    border-radius: 4px;
    background: color-mix(in srgb, var(--afs-line) 60%, transparent);
    color: var(--afs-ink);
  }
`;

const FirstPrompt = styled.blockquote`
  margin: 0;
  padding: 10px 14px;
  border-left: 3px solid var(--afs-accent, #2563eb);
  background: color-mix(in srgb, var(--afs-accent, #2563eb) 8%, transparent);
  color: var(--afs-ink);
  font-size: 14px;
  line-height: 1.55;
  font-style: italic;
`;
