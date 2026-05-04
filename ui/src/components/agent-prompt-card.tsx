import { Check, Copy } from "lucide-react";
import { useState } from "react";
import type { ReactNode } from "react";
import styled from "styled-components";

type Tone = "primary" | "secondary";

type Props = {
  eyebrow?: string;
  title: ReactNode;
  description?: ReactNode;
  prompt: string;
  tone?: Tone;
  footer?: ReactNode;
};

export function AgentPromptCard({
  eyebrow,
  title,
  description,
  prompt,
  tone = "secondary",
  footer,
}: Props) {
  const [copied, setCopied] = useState(false);

  async function copyPrompt() {
    try {
      await navigator.clipboard.writeText(prompt);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      /* clipboard unavailable */
    }
  }

  return (
    <Card $tone={tone}>
      <Header>
        {eyebrow ? <Eyebrow $tone={tone}>{eyebrow}</Eyebrow> : null}
        <Title>{title}</Title>
        {description ? <Description>{description}</Description> : null}
      </Header>
      <PromptBlock>
        <PromptScroll>
          <PromptText>{prompt}</PromptText>
        </PromptScroll>
        <CopyButton type="button" onClick={copyPrompt} $tone={tone} aria-label="Copy prompt">
          {copied ? <Check size={16} strokeWidth={2.2} /> : <Copy size={16} strokeWidth={2.2} />}
          <span>{copied ? "Copied" : "Copy prompt"}</span>
        </CopyButton>
      </PromptBlock>
      {footer ? <Footer>{footer}</Footer> : null}
    </Card>
  );
}

const Card = styled.section<{ $tone: Tone }>`
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 22px 24px 22px;
  border-radius: 16px;
  border: 1px solid
    ${(p) =>
      p.$tone === "primary"
        ? "color-mix(in srgb, var(--afs-accent) 35%, var(--afs-line))"
        : "var(--afs-line)"};
  background: ${(p) =>
    p.$tone === "primary"
      ? "linear-gradient(180deg, color-mix(in srgb, var(--afs-accent) 4%, var(--afs-panel-strong)), var(--afs-panel-strong))"
      : "var(--afs-panel)"};
  box-shadow: ${(p) =>
    p.$tone === "primary"
      ? "0 12px 32px color-mix(in srgb, var(--afs-accent) 16%, transparent)"
      : "none"};
`;

const Header = styled.div`
  display: flex;
  flex-direction: column;
  gap: 6px;
`;

const Eyebrow = styled.div<{ $tone: Tone }>`
  color: ${(p) => (p.$tone === "primary" ? "var(--afs-accent)" : "var(--afs-muted)")};
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 0.14em;
  text-transform: uppercase;
`;

const Title = styled.h3`
  margin: 0;
  color: var(--afs-ink);
  font-size: 20px;
  font-weight: 750;
  line-height: 1.25;
  letter-spacing: -0.01em;
`;

const Description = styled.p`
  margin: 0;
  color: var(--afs-muted);
  font-size: 14px;
  line-height: 1.55;
`;

const PromptBlock = styled.div`
  position: relative;
  border: 1px solid var(--afs-line);
  border-radius: 12px;
  background: #0f1720;
  overflow: hidden;
`;

const PromptScroll = styled.div`
  max-height: 360px;
  overflow: auto;
  padding: 16px 18px 56px;
`;

const PromptText = styled.pre`
  margin: 0;
  color: #e6edf3;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12.5px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
`;

const CopyButton = styled.button<{ $tone: Tone }>`
  position: absolute;
  bottom: 10px;
  right: 10px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 8px;
  border: 1px solid
    ${(p) =>
      p.$tone === "primary"
        ? "var(--afs-accent)"
        : "rgba(255, 255, 255, 0.18)"};
  background: ${(p) =>
    p.$tone === "primary"
      ? "var(--afs-accent)"
      : "rgba(255, 255, 255, 0.06)"};
  color: ${(p) =>
    p.$tone === "primary" ? "var(--afs-ink-on-accent, #fff)" : "#e6edf3"};
  font-family: var(--afs-mono, "SF Mono", "Fira Code", monospace);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  cursor: pointer;
  transition: background 140ms ease, border-color 140ms ease;

  &:hover {
    background: ${(p) =>
      p.$tone === "primary"
        ? "color-mix(in srgb, var(--afs-accent) 88%, white)"
        : "rgba(255, 255, 255, 0.12)"};
  }
`;

const Footer = styled.div`
  color: var(--afs-muted);
  font-size: 13px;
  line-height: 1.55;
`;
