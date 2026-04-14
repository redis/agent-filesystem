import styled, { keyframes, css } from "styled-components";
import { RedisLogoDarkMinIcon } from "@redislabsdev/redis-ui-icons/multicolor";

/* ------------------------------------------------------------------ */
/*  Animated hero: agents <-> Redis hub <-> filesystem workspaces      */
/* ------------------------------------------------------------------ */

const AGENTS = [
  { label: "Claude", icon: "C", hue: 262 },
  { label: "GPT Agent", icon: "G", hue: 160 },
  { label: "Custom Bot", icon: "B", hue: 30 },
];

const WORKSPACES = [
  { label: "customer-portal", files: 142 },
  { label: "ml-pipeline", files: 87 },
  { label: "docs-site", files: 56 },
];

/* ---- Keyframes ---- */
const fadeInUp = keyframes`
  from { opacity: 0; transform: translateY(12px); }
  to   { opacity: 1; transform: translateY(0); }
`;

const pulseGlow = keyframes`
  0%, 100% { box-shadow: 0 0 0 0 rgba(220, 38, 38, 0.2); }
  50%      { box-shadow: 0 0 20px 4px rgba(220, 38, 38, 0.15); }
`;

const floatY = keyframes`
  0%, 100% { transform: translateY(0); }
  50%      { transform: translateY(-3px); }
`;

const marchLeft = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: -16; }
`;

const marchRight = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: 16; }
`;

const dotTravel = keyframes`
  0%   { offset-distance: 0%;   opacity: 0; }
  8%   { opacity: 1; }
  92%  { opacity: 1; }
  100% { offset-distance: 100%; opacity: 0; }
`;

/* ---- Layout ---- */
const HeroWrap = styled.div`
  position: relative;
  width: 100%;
  padding: 12px 0 8px;
`;

const Diagram = styled.div`
  display: grid;
  grid-template-columns: minmax(140px, 180px) 48px 80px 48px minmax(140px, 200px);
  align-items: center;
  justify-content: center;
  gap: 0;
  min-height: 180px;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
    grid-template-rows: auto auto auto;
    gap: 16px;

    & > :nth-child(2),
    & > :nth-child(4) {
      display: none;
    }
  }
`;

/* ---- Column wrappers ---- */
const Column = styled.div<{ $align?: string }>`
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: ${({ $align }) => $align ?? "stretch"};
`;

const ColumnLabel = styled.div`
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--afs-muted, #71717a);
  margin-bottom: 2px;
  text-align: center;
`;

/* ---- Agent nodes ---- */
const AgentNode = styled.div<{ $i: number }>`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel-strong);
  opacity: 0;
  animation:
    ${fadeInUp} 0.4s ease forwards,
    ${floatY} ${({ $i }) => 3 + $i * 0.4}s ease-in-out infinite;
  animation-delay: ${({ $i }) => $i * 0.1}s, ${({ $i }) => $i * 0.25}s;
`;

const AgentIcon = styled.div<{ $hue: number }>`
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 800;
  color: #fff;
  background: ${({ $hue }) => `hsl(${$hue}, 72%, 52%)`};
  flex-shrink: 0;
`;

const AgentLabel = styled.span`
  font-size: 12px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
`;

/* ---- Hub ---- */
const HubNode = styled.div`
  width: 80px;
  height: 80px;
  border-radius: 20px;
  background: linear-gradient(135deg, #dc2626 0%, #ef4444 100%);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  color: #fff;
  animation: ${pulseGlow} 3s ease-in-out infinite;
  justify-self: center;
`;

const HubLabel = styled.span`
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  opacity: 0.9;
`;

/* ---- Connection lines with animated dots ---- */
const LinesColumn = styled.div`
  position: relative;
  width: 48px;
  height: 100%;
  min-height: 160px;

  @media (max-width: 720px) {
    display: none;
  }
`;

const LinesSvg = styled.svg`
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  overflow: visible;
`;

const DashedLine = styled.line<{ $toRight?: boolean }>`
  stroke: var(--afs-line, #d4d4d8);
  stroke-width: 1.5;
  stroke-dasharray: 4 4;
  animation: ${({ $toRight }) => ($toRight ? marchRight : marchLeft)} 1s linear infinite;
`;

/* Animated dot that travels along a path */
const TravelDot = styled.circle`
  fill: currentColor;
  filter: drop-shadow(0 0 3px currentColor);
`;

/* ---- Workspace nodes ---- */
const WorkspaceNode = styled.div<{ $i: number }>`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel-strong);
  opacity: 0;
  animation:
    ${fadeInUp} 0.4s ease forwards,
    ${floatY} ${({ $i }) => 3.2 + $i * 0.5}s ease-in-out infinite;
  animation-delay: ${({ $i }) => 0.25 + $i * 0.1}s, ${({ $i }) => 0.4 + $i * 0.3}s;
`;

const FolderIcon = styled.div`
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  background: var(--afs-accent-soft);
  color: var(--afs-accent);
  flex-shrink: 0;
`;

const WorkspaceMeta = styled.div`
  display: flex;
  flex-direction: column;
`;

const WorkspaceName = styled.span`
  font-size: 12px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
`;

const WorkspaceFiles = styled.span`
  font-size: 10px;
  color: var(--afs-muted, #71717a);
`;

/* ------------------------------------------------------------------ */
/*  Animated connection lines component using SVG <animateMotion>      */
/* ------------------------------------------------------------------ */

function ConnectionLines({
  nodeCount,
  toRight,
  color,
}: {
  nodeCount: number;
  toRight?: boolean;
  color: string;
}) {
  // Compute y positions for each node (evenly spaced within the SVG)
  const svgH = 160;
  const hubY = svgH / 2;
  const spacing = svgH / (nodeCount + 1);

  return (
    <LinesColumn>
      <LinesSvg viewBox={`0 0 48 ${svgH}`} preserveAspectRatio="none">
        {Array.from({ length: nodeCount }).map((_, i) => {
          const nodeY = spacing * (i + 1);
          const x1 = toRight ? 0 : 48;
          const x2 = toRight ? 48 : 0;

          // Path for the dot to travel along
          const pathD = `M ${x1} ${hubY} L ${x2} ${nodeY}`;
          const pathBackD = `M ${x2} ${nodeY} L ${x1} ${hubY}`;

          return (
            <g key={i}>
              <DashedLine
                x1={x1}
                y1={hubY}
                x2={x2}
                y2={nodeY}
                $toRight={toRight}
              />
              {/* Dot traveling outward */}
              <TravelDot r="3" style={{ color }}>
                <animateMotion
                  path={pathD}
                  dur={`${1.8 + i * 0.3}s`}
                  begin={`${i * 0.5}s`}
                  repeatCount="indefinite"
                  calcMode="linear"
                />
              </TravelDot>
              {/* Dot traveling inward */}
              <TravelDot r="2.5" style={{ color }} opacity="0.6">
                <animateMotion
                  path={pathBackD}
                  dur={`${2.2 + i * 0.25}s`}
                  begin={`${0.8 + i * 0.4}s`}
                  repeatCount="indefinite"
                  calcMode="linear"
                />
              </TravelDot>
            </g>
          );
        })}
      </LinesSvg>
    </LinesColumn>
  );
}

/* ------------------------------------------------------------------ */
/*  Main component                                                     */
/* ------------------------------------------------------------------ */
export function AgentHeroAnimation() {
  return (
    <HeroWrap>
      <Diagram>
        {/* ── Left: Agents ── */}
        <Column $align="flex-end">
          <ColumnLabel>AI Agents</ColumnLabel>
          {AGENTS.map((a, i) => (
            <AgentNode key={a.label} $i={i}>
              <AgentIcon $hue={a.hue}>{a.icon}</AgentIcon>
              <AgentLabel>{a.label}</AgentLabel>
            </AgentNode>
          ))}
        </Column>

        {/* ── Left lines ── */}
        <ConnectionLines nodeCount={AGENTS.length} toRight color="#a78bfa" />

        {/* ── Center: Redis Hub ── */}
        <HubNode>
          <RedisLogoDarkMinIcon customSize="36px" style={{ filter: "brightness(0) invert(1)" }} />
          <HubLabel>Redis</HubLabel>
        </HubNode>

        {/* ── Right lines ── */}
        <ConnectionLines nodeCount={WORKSPACES.length} color="#818cf8" />

        {/* ── Right: Workspaces ── */}
        <Column $align="flex-start">
          <ColumnLabel>Workspaces</ColumnLabel>
          {WORKSPACES.map((w, i) => (
            <WorkspaceNode key={w.label} $i={i}>
              <FolderIcon>&#128193;</FolderIcon>
              <WorkspaceMeta>
                <WorkspaceName>{w.label}</WorkspaceName>
                <WorkspaceFiles>{w.files} files</WorkspaceFiles>
              </WorkspaceMeta>
            </WorkspaceNode>
          ))}
        </Column>
      </Diagram>
    </HeroWrap>
  );
}
