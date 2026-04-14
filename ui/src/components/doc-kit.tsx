import styled, { css } from "styled-components";

/* ------------------------------------------------------------------ */
/*  Shared components for documentation / guide pages                  */
/* ------------------------------------------------------------------ */

const panelSurface = css`
  border: 1px solid var(--afs-line);
  border-radius: 16px;
  background: var(--afs-panel-strong);
`;

/* ---- Page wrapper (narrower for readability) ---- */
export const DocPage = styled.div`
  display: flex;
  flex-direction: column;
  gap: 24px;
  width: min(100%, 960px);
  margin: 0 auto;
  padding: 28px 32px 44px;

  @media (max-width: 900px) {
    padding: 20px 18px 36px;
  }
`;

/* ---- Section card ---- */
export const DocSection = styled.div`
  ${panelSurface}
  padding: 28px;

  @media (max-width: 720px) {
    padding: 20px;
  }
`;

/* ---- Typography ---- */
export const DocHeading = styled.h3`
  margin: 0 0 6px;
  color: var(--afs-ink, #18181b);
  font-size: 18px;
  font-weight: 700;
  line-height: 1.35;
  letter-spacing: -0.01em;
`;

export const DocSubheading = styled.h4`
  margin: 20px 0 6px;
  color: var(--afs-ink, #18181b);
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
`;

export const DocProse = styled.p`
  margin: 0;
  color: var(--afs-muted, #71717a);
  font-size: 14px;
  line-height: 1.7;

  & + & {
    margin-top: 12px;
  }
`;

/* ---- Code blocks ---- */
export const CodeBlock = styled.pre`
  ${panelSurface}
  margin: 12px 0 0;
  padding: 16px 20px;
  overflow-x: auto;
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 13px;
  line-height: 1.6;
  color: var(--afs-ink, #18181b);
  background: var(--afs-panel);

  code {
    font: inherit;
    color: inherit;
  }
`;

export const InlineCode = styled.code`
  padding: 2px 7px;
  border-radius: 6px;
  background: var(--afs-panel);
  font-family: var(--afs-mono, "SF Mono", "Fira Code", "Cascadia Code", monospace);
  font-size: 0.88em;
  color: var(--afs-ink-soft, #40384d);
`;

/* ---- Callout / tip box ---- */
export const CalloutBox = styled.div<{ $tone?: "info" | "tip" | "warn" }>`
  ${panelSurface}
  margin: 14px 0 0;
  padding: 16px 20px;
  border-left: 3px solid
    ${({ $tone = "info" }) =>
      $tone === "tip"
        ? "#22c55e"
        : $tone === "warn"
          ? "#f59e0b"
          : "var(--afs-accent, #6366f1)"};
  background: ${({ $tone = "info" }) =>
    $tone === "tip"
      ? "rgba(220,255,49,0.09)"
      : $tone === "warn"
        ? "rgba(245,158,11,0.04)"
        : "var(--afs-accent-soft)"};
`;

/* ---- Step card (numbered walkthrough) ---- */
const StepWrap = styled.div`
  display: flex;
  gap: 16px;
  align-items: flex-start;

  & + & {
    margin-top: 20px;
    padding-top: 20px;
    border-top: 1px solid var(--afs-line, #e4e4e7);
  }
`;

const StepNumber = styled.div`
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 800;
  color: var(--afs-accent, #6366f1);
  background: var(--afs-accent-soft, rgba(99, 102, 241, 0.1));
`;

const StepBody = styled.div`
  flex: 1;
  min-width: 0;
`;

const StepTitle = styled.div`
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  margin-bottom: 4px;
`;

const StepDesc = styled.div`
  color: var(--afs-muted, #71717a);
  font-size: 13px;
  line-height: 1.65;
`;

export function Step(props: {
  n: number;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <StepWrap>
      <StepNumber>{String(props.n).padStart(2, "0")}</StepNumber>
      <StepBody>
        <StepTitle>{props.title}</StepTitle>
        <StepDesc>{props.children}</StepDesc>
      </StepBody>
    </StepWrap>
  );
}

/* ---- Command table ---- */
export const CmdTable = styled.table`
  width: 100%;
  border-collapse: collapse;
  margin: 12px 0 0;
  font-size: 13px;

  th {
    text-align: left;
    padding: 8px 12px;
    font-weight: 700;
    color: var(--afs-muted, #71717a);
    font-size: 11px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    border-bottom: 1px solid var(--afs-line, #e4e4e7);
  }

  td {
    padding: 8px 12px;
    color: var(--afs-ink, #18181b);
    border-bottom: 1px solid var(--afs-line, #e4e4e7);
    vertical-align: top;
  }

  tr:last-child td {
    border-bottom: none;
  }
`;

/* ---- Cross-link card ---- */
export const CrossLinkCard = styled.a`
  ${panelSurface}
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 20px 24px;
  text-decoration: none;
  transition: border-color 180ms ease, transform 180ms ease;

  &:hover {
    border-color: var(--afs-accent, #6366f1);
    transform: translateY(-1px);
  }
`;

export const CrossLinkText = styled.div`
  flex: 1;
`;

export const CrossLinkTitle = styled.div`
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
`;

export const CrossLinkDesc = styled.div`
  font-size: 13px;
  color: var(--afs-muted, #71717a);
  margin-top: 2px;
`;

export const CrossLinkArrow = styled.span`
  font-size: 18px;
  color: var(--afs-accent, #6366f1);
`;

/* ---- Prominent raw-file link ---- */
export const RawFileLink = styled.a`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 10px 18px;
  border: 1px solid var(--afs-accent, #6366f1);
  border-radius: 10px;
  background: var(--afs-accent-soft, rgba(99, 102, 241, 0.08));
  color: var(--afs-accent, #6366f1);
  font-size: 13px;
  font-weight: 700;
  text-decoration: none;
  transition: background 180ms ease, transform 180ms ease;

  &:hover {
    background: color-mix(in srgb, var(--afs-accent-soft) 75%, var(--afs-panel) 25%);
    transform: translateY(-1px);
  }
`;

/* ---- Hero section for page top ---- */
export const DocHero = styled.div`
  margin-bottom: 4px;
`;

export const DocHeroTitle = styled.h2`
  margin: 0;
  color: var(--afs-ink, #18181b);
  font-size: clamp(1.4rem, 2.5vw, 1.75rem);
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.02em;
`;

export const DocHeroSub = styled.p`
  margin: 8px 0 0;
  color: var(--afs-muted, #71717a);
  font-size: 15px;
  line-height: 1.6;
  max-width: 640px;
`;
