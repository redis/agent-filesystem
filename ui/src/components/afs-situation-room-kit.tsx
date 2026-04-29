/**
 * AFS · Situation Room primitives
 *
 * Skin-native components inspired by the Situation Room HTML reference
 * (Agent Filesystem Coder Edition/styles.css). They render usefully under
 * the classic skin too, but their visual identity belongs to situation-room.
 *
 * Composition shapes:
 *   <Panel><PanelHead title="…" /><PanelBody>…</PanelBody></Panel>
 *   <Term><TermLine>…</TermLine></Term>
 *   <SectionHead num="01" title="…" meta="…" />
 *   <KbdKey>⌘K</KbdKey>
 *   <Spark bars={5} />
 */
import type { ReactNode } from "react";
import styled from "styled-components";

/* ── Panel + PanelHead ─────────────────────────────────────────── */

export const Panel = styled.div`
  border: 1px solid var(--afs-line-strong, var(--afs-line));
  border-radius: 4px;
  background: var(--afs-bg-1, var(--afs-panel-strong));
  position: relative;
  overflow: hidden;

  [data-skin="situation-room"] && {
    border-radius: var(--afs-r-2);
  }
`;

const PanelHeadRow = styled.div`
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--afs-line-strong, var(--afs-line));
  background: var(--afs-bg-2, var(--afs-panel));
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-xs, 11px);
  letter-spacing: 0.1em;
  color: var(--afs-ink-dim, var(--afs-muted));
  text-transform: uppercase;
`;

const Dots = styled.div`
  display: flex;
  gap: 6px;
`;

const Dot = styled.span<{ $tone?: "g" | "a" | "r" | "muted" }>`
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: ${({ $tone = "muted" }) =>
    $tone === "g"
      ? "var(--afs-ok, #DCFF1E)"
      : $tone === "a"
        ? "var(--afs-warn, #FFB547)"
        : $tone === "r"
          ? "var(--afs-err, #FF4D4D)"
          : "var(--afs-line-strong, #284A5E)"};
`;

const PanelHeadTitle = styled.div`
  color: var(--afs-ink);
  font-weight: 500;
  text-align: center;
  letter-spacing: 0.05em;
`;

const PanelHeadMeta = styled.div`
  text-align: right;
  color: var(--afs-ink-dim, var(--afs-muted));
`;

export function PanelHead(props: { title?: string; meta?: ReactNode; dots?: boolean }) {
  return (
    <PanelHeadRow>
      {props.dots !== false ? (
        <Dots aria-hidden="true">
          <Dot $tone="g" />
          <Dot $tone="a" />
          <Dot $tone="r" />
        </Dots>
      ) : (
        <span />
      )}
      <PanelHeadTitle>{props.title ?? ""}</PanelHeadTitle>
      <PanelHeadMeta>{props.meta ?? ""}</PanelHeadMeta>
    </PanelHeadRow>
  );
}

export const PanelBody = styled.div`
  padding: 16px;
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-sm, 12px);
  color: var(--afs-ink);
`;

/* ── Terminal block ────────────────────────────────────────────── */

export const Term = styled.div`
  padding: 16px 18px;
  background: var(--afs-bg, #091a23);
  color: var(--afs-ink);
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-sm, 12px);
  line-height: 1.65;
`;

export const TermLine = styled.div`
  white-space: pre-wrap;
`;

export const TermPrompt = styled.span`
  color: var(--afs-accent, #DCFF1E);
  user-select: none;
  margin-right: 8px;
`;

export const TermOut = styled.span`
  color: var(--afs-ink-dim, var(--afs-muted));
`;

export const TermOk = styled.span`
  color: var(--afs-ok, #DCFF1E);
`;

export const TermWarn = styled.span`
  color: var(--afs-warn, #FFB547);
`;

export const TermErr = styled.span`
  color: var(--afs-err, #FF4D4D);
`;

export const TermCmt = styled.span`
  color: var(--afs-ink-faint, #4A4C48);
`;

export const TermCursor = styled.span`
  display: inline-block;
  width: 8px;
  height: 1em;
  background: var(--afs-accent, #DCFF1E);
  vertical-align: -2px;
  margin-left: 2px;
  animation: afs-blink 1s steps(2, end) infinite;
`;

/* ── Section header (eyebrow + title + meta) ───────────────────── */

const SectionHeadRow = styled.div`
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: end;
  gap: 24px;
  padding: 18px 0 14px;
  border-bottom: 1px solid var(--afs-line);
  margin-bottom: 24px;

  @media (max-width: 720px) {
    grid-template-columns: auto 1fr;
  }
`;

const SectionHeadNum = styled.div`
  color: var(--afs-accent);
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-xs, 11px);
  letter-spacing: 0.2em;
`;

const SectionHeadTitle = styled.div`
  color: var(--afs-ink);
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-lg, 15px);
  font-weight: 500;
  letter-spacing: 0.04em;
`;

const SectionHeadMeta = styled.div`
  color: var(--afs-ink-dim, var(--afs-muted));
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-xs, 11px);
  letter-spacing: 0.1em;
  text-align: right;

  @media (max-width: 720px) {
    grid-column: 1 / -1;
    text-align: left;
  }
`;

export function SectionHead(props: { num?: string; title: string; meta?: ReactNode }) {
  return (
    <SectionHeadRow>
      <SectionHeadNum>{props.num ?? ""}</SectionHeadNum>
      <SectionHeadTitle>{props.title}</SectionHeadTitle>
      {props.meta ? <SectionHeadMeta>{props.meta}</SectionHeadMeta> : <span />}
    </SectionHeadRow>
  );
}

/* ── KbdKey ────────────────────────────────────────────────────── */

export const KbdKey = styled.kbd`
  display: inline-block;
  padding: 2px 6px;
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: 10px;
  border: 1px solid var(--afs-line-strong, var(--afs-line));
  color: var(--afs-ink-dim, var(--afs-muted));
  background: var(--afs-bg-1, var(--afs-panel));
  border-radius: 2px;
  letter-spacing: 0.04em;
`;

/* ── Spark (animated mini bar chart) ───────────────────────────── */

const SparkRow = styled.span`
  display: inline-flex;
  align-items: end;
  gap: 2px;
  height: 14px;
`;

const SparkBar = styled.span<{ $delay: number }>`
  display: inline-block;
  width: 3px;
  background: var(--afs-accent, #DCFF1E);
  height: 60%;
  animation: afs-spark 1.6s ease-in-out infinite;
  animation-delay: ${({ $delay }) => `${$delay}ms`};
`;

export function Spark(props: { bars?: number }) {
  const count = Math.max(2, Math.min(props.bars ?? 5, 12));
  return (
    <SparkRow aria-hidden="true">
      {Array.from({ length: count }, (_, i) => (
        <SparkBar key={i} $delay={i * 80} />
      ))}
    </SparkRow>
  );
}

/* ── Eyebrow (small uppercase accent label) ────────────────────── */

export const Eyebrow = styled.span`
  font-family: var(--afs-font-mono, var(--afs-mono));
  font-size: var(--afs-fz-xs, 11px);
  color: var(--afs-accent);
  letter-spacing: 0.2em;
  text-transform: uppercase;
`;

/* ── Brackets (decorative around text) ─────────────────────────── */

export function Brackets(props: { children: ReactNode }) {
  return (
    <span>
      <BracketSpan>[</BracketSpan>
      {props.children}
      <BracketSpan>]</BracketSpan>
    </span>
  );
}

const BracketSpan = styled.span`
  color: var(--afs-accent);
  font-family: var(--afs-font-mono, var(--afs-mono));
  margin: 0 4px;
`;

/* ── LED dot (status indicator) ────────────────────────────────── */

export const Led = styled.span<{ $tone?: "ok" | "warn" | "err" | "info" }>`
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: ${({ $tone = "ok" }) =>
    $tone === "warn"
      ? "var(--afs-warn)"
      : $tone === "err"
        ? "var(--afs-err)"
        : $tone === "info"
          ? "var(--afs-info)"
          : "var(--afs-accent)"};
  box-shadow: 0 0 6px currentColor;
  animation: afs-blink var(--afs-dur-tick, 1400ms) steps(2, end) infinite;
  flex-shrink: 0;
`;
