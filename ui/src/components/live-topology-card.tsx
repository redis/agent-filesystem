import { useEffect, useMemo, useRef, useLayoutEffect, useState, useCallback } from "react";
import styled, { keyframes } from "styled-components";
import { RedisLogoDarkMinIcon } from "@redis-ui/icons/multicolor";
import type { AFSAgentSession, AFSWorkspaceSummary } from "../foundation/types/afs";

/* ------------------------------------------------------------------ */
/*  Live topology: agents <-> Redis hub <-> workspaces                 */
/* ------------------------------------------------------------------ */

/* ---- Keyframes ---- */
const pulseGlow = keyframes`
  0%, 100% { box-shadow: 0 0 0 0 rgba(220, 38, 38, 0.2); }
  50%      { box-shadow: 0 0 20px 4px rgba(220, 38, 38, 0.15); }
`;

const fadeIn = keyframes`
  from { opacity: 0; }
  to   { opacity: 1; }
`;

const marchLeft = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: -16; }
`;

const marchRight = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: 16; }
`;

/* ---- Outer card ---- */
const CardWrap = styled.div`
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  padding: 24px;
`;

const CardTitle = styled.h3`
  margin: 0 0 4px;
  font-size: 14px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
`;

const CardSubtitle = styled.p`
  margin: 0 0 20px;
  font-size: 13px;
  color: var(--afs-muted, #71717a);
  line-height: 1.5;
`;

/* ---- 3-column layout ---- */
const Topology = styled.div`
  display: grid;
  grid-template-columns: minmax(140px, 220px) 1fr minmax(140px, 220px);
  align-items: start;
  gap: 0;
  min-height: 160px;
  position: relative;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
    gap: 16px;
  }
`;

/* ---- Column wrappers ---- */
const Column = styled.div<{ $align?: string }>`
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: ${({ $align }) => $align ?? "stretch"};
  z-index: 1;
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
const AgentNode = styled.button<{ $i: number; $canCopy: boolean; $copied: boolean }>`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid ${({ $copied }) => ($copied ? "#22c55e" : "var(--afs-line, #e4e4e7)")};
  border-radius: 10px;
  background: var(--afs-panel-strong);
  color: inherit;
  cursor: ${({ $canCopy }) => ($canCopy ? "copy" : "default")};
  font: inherit;
  text-align: left;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease;
  animation: ${fadeIn} 0.24s ease forwards;
  animation-delay: ${({ $i }) => $i * 0.06}s;
  opacity: 0;

  &:hover {
    border-color: ${({ $canCopy, $copied }) =>
      $copied ? "#22c55e" : $canCopy ? "var(--afs-accent, #dc2626)" : "var(--afs-line, #e4e4e7)"};
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 2px;
  }
`;

const AgentIcon = styled.div<{ $hue: number }>`
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 800;
  color: #fff;
  background: ${({ $hue }) => `hsl(${$hue}, 72%, 52%)`};
  flex-shrink: 0;
`;

const AgentLabel = styled.span`
  font-size: 12px;
  font-weight: 800;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
`;

const AgentText = styled.div`
  display: flex;
  flex-direction: column;
  min-width: 0;
`;

const AgentMeta = styled.span`
  font-size: 10px;
  color: var(--afs-muted, #71717a);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
`;

const AgentPath = styled.span`
  max-width: 130px;
  overflow: hidden;
  color: var(--afs-muted, #71717a);
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 10px;
  text-overflow: ellipsis;
  white-space: nowrap;
`;

/* ---- Hub ---- */
const HubWrap = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  align-self: center;
  z-index: 2;

  @media (max-width: 720px) {
    display: none;
  }
`;

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
  flex-shrink: 0;
`;

const HubLabel = styled.span`
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  opacity: 0.9;
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
  animation: ${fadeIn} 0.24s ease forwards;
  animation-delay: ${({ $i }) => 0.18 + $i * 0.06}s;
  opacity: 0;
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
  min-width: 0;
`;

const WorkspaceName = styled.span`
  font-size: 12px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`;

const WorkspaceFiles = styled.span`
  font-size: 10px;
  color: var(--afs-muted, #71717a);
`;

/* ---- SVG overlay for connection lines ---- */
const SvgOverlay = styled.svg`
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  overflow: visible;
  z-index: 0;

  @media (max-width: 720px) {
    display: none;
  }
`;

const DashedLine = styled.line<{ $toRight?: boolean }>`
  stroke: var(--afs-line, #d4d4d8);
  stroke-width: 1.5;
  stroke-dasharray: 4 4;
  animation: ${({ $toRight }) => ($toRight ? marchRight : marchLeft)} 1s linear
    infinite;
`;

const TravelDot = styled.circle`
  fill: currentColor;
  filter: drop-shadow(0 0 3px currentColor);
`;

/* ---- Empty placeholder ---- */
const EmptyColumn = styled.div`
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100px;
  border: 1px dashed var(--afs-line, #e4e4e7);
  border-radius: 10px;
  padding: 16px;
  color: var(--afs-muted, #71717a);
  font-size: 12px;
  text-align: center;
`;

/* ---- Status dot ---- */
const StatusDot = styled.span<{ $active: boolean }>`
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: ${({ $active }) => ($active ? "#22c55e" : "#d4d4d8")};
  flex-shrink: 0;
`;

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

const CLIENT_KIND_MAP: Record<string, { icon: string; hue: number }> = {
  claude: { icon: "C", hue: 262 },
  "claude-code": { icon: "C", hue: 262 },
  openai: { icon: "G", hue: 160 },
  gpt: { icon: "G", hue: 160 },
  custom: { icon: "B", hue: 30 },
};

function agentVisual(clientKind: string): { icon: string; hue: number } {
  const lower = clientKind.toLowerCase();
  for (const [key, val] of Object.entries(CLIENT_KIND_MAP)) {
    if (lower.includes(key)) return val;
  }
  // Hash the string to a hue for unknown agents
  let hash = 0;
  for (let i = 0; i < lower.length; i++) {
    hash = lower.charCodeAt(i) + ((hash << 5) - hash);
  }
  const hue = ((hash % 360) + 360) % 360;
  return { icon: lower === "" ? "A" : lower[0].toUpperCase(), hue };
}

function displayLocalPath(path: string): string {
  return path.trim().replace(/^\/Users\/[^/]+\/?/, "~/");
}

function displayAgentName(agent: AFSAgentSession): string {
  return (
    agent.label?.trim() ||
    agent.agentId?.trim() ||
    agent.hostname.trim() ||
    "agent"
  );
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

type Props = {
  agents: AFSAgentSession[];
  workspaces: AFSWorkspaceSummary[];
};

export function LiveTopologyCard({ agents, workspaces }: Props) {
  const topologyRef = useRef<HTMLDivElement>(null);
  const agentRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const wsRefs = useRef<(HTMLDivElement | null)[]>([]);
  const hubRef = useRef<HTMLDivElement>(null);
  const animationFrameRef = useRef<number | null>(null);
  const copyResetTimeoutRef = useRef<number | null>(null);
  const [copiedAgentId, setCopiedAgentId] = useState<string | null>(null);
  const [lines, setLines] = useState<
    {
      x1: number;
      y1: number;
      x2: number;
      y2: number;
      agentIdx: number;
      wsIdx: number;
    }[]
  >([]);

  // Build a map: workspaceId -> index in the displayed workspace list
  const wsIndexMap = useMemo(() => {
    const map = new Map<string, number>();
    workspaces.forEach((ws, i) => map.set(ws.id, i));
    return map;
  }, [workspaces]);

  // Build connection pairs: [agentIdx, wsIdx]
  const connections = useMemo(() => {
    const pairs: { agentIdx: number; wsIdx: number }[] = [];
    const seen = new Set<string>();
    agents.forEach((agent, aIdx) => {
      const wIdx = wsIndexMap.get(agent.workspaceId);
      if (wIdx != null) {
        const key = `${aIdx}-${wIdx}`;
        if (!seen.has(key)) {
          seen.add(key);
          pairs.push({ agentIdx: aIdx, wsIdx: wIdx });
        }
      }
    });
    return pairs;
  }, [agents, wsIndexMap]);

  const copyAgentId = useCallback(async (agentId: string) => {
    if (!agentId) {
      return;
    }

    try {
      await navigator.clipboard.writeText(agentId);
      setCopiedAgentId(agentId);
      if (copyResetTimeoutRef.current != null) {
        window.clearTimeout(copyResetTimeoutRef.current);
      }
      copyResetTimeoutRef.current = window.setTimeout(() => {
        setCopiedAgentId(null);
        copyResetTimeoutRef.current = null;
      }, 1400);
    } catch {
      // Clipboard permissions are browser-controlled; ignore denied writes.
    }
  }, []);

  const computeLines = useCallback(() => {
    const container = topologyRef.current;
    const hub = hubRef.current;
    if (!container || !hub) return;

    if (connections.length === 0) {
      setLines([]);
      return;
    }

    const cRect = container.getBoundingClientRect();
    const hRect = hub.getBoundingClientRect();
    const hubCx = hRect.left + hRect.width / 2 - cRect.left;
    const hubCy = hRect.top + hRect.height / 2 - cRect.top;

    const newLines: typeof lines = [];

    // Draw only explicit agent -> workspace connections through the hub.
    connections.forEach(({ agentIdx, wsIdx }) => {
      const aEl = agentRefs.current[agentIdx];
      const wEl = wsRefs.current[wsIdx];
      if (!aEl || !wEl) return;

      const aRect = aEl.getBoundingClientRect();
      const wRect = wEl.getBoundingClientRect();

      newLines.push({
        x1: aRect.right - cRect.left,
        y1: aRect.top + aRect.height / 2 - cRect.top,
        x2: hubCx,
        y2: hubCy,
        agentIdx,
        wsIdx,
      });
      newLines.push({
        x1: hubCx,
        y1: hubCy,
        x2: wRect.left - cRect.left,
        y2: wRect.top + wRect.height / 2 - cRect.top,
        agentIdx,
        wsIdx,
      });
    });

    setLines(newLines);
  }, [connections]);

  const scheduleLineCompute = useCallback(() => {
    if (animationFrameRef.current != null) {
      cancelAnimationFrame(animationFrameRef.current);
    }
    animationFrameRef.current = requestAnimationFrame(() => {
      animationFrameRef.current = null;
      computeLines();
    });
  }, [computeLines]);

  useLayoutEffect(() => {
    const resizeObserver =
      typeof ResizeObserver === "undefined"
        ? null
        : new ResizeObserver(() => {
            scheduleLineCompute();
          });

    const observedElements = [
      topologyRef.current,
      hubRef.current,
      ...agentRefs.current.slice(0, agents.length),
      ...wsRefs.current.slice(0, workspaces.length),
    ].filter((element): element is HTMLElement => element != null);

    observedElements.forEach((element) => resizeObserver?.observe(element));

    scheduleLineCompute();
    window.addEventListener("resize", scheduleLineCompute);

    return () => {
      window.removeEventListener("resize", scheduleLineCompute);
      resizeObserver?.disconnect();
      if (animationFrameRef.current != null) {
        cancelAnimationFrame(animationFrameRef.current);
        animationFrameRef.current = null;
      }
    };
  }, [agents.length, workspaces.length, scheduleLineCompute]);

  useEffect(() => {
    return () => {
      if (copyResetTimeoutRef.current != null) {
        window.clearTimeout(copyResetTimeoutRef.current);
      }
    };
  }, []);

  const activeAgents = agents.filter((a) => a.state === "active").length;

  return (
    <CardWrap>
      <CardTitle>Live Topology</CardTitle>
      <CardSubtitle>
        {agents.length === 0 && workspaces.length === 0
          ? "Connect agents and create workspaces to see them here."
          : `${agents.length} agent${agents.length === 1 ? "" : "s"} connected${activeAgents > 0 ? ` (${activeAgents} active)` : ""} \u00B7 ${workspaces.length} workspace${workspaces.length === 1 ? "" : "s"}`}
      </CardSubtitle>

      <Topology ref={topologyRef}>
        {/* SVG lines overlay */}
        <SvgOverlay>
          {lines.map((l, i) => {
            const isLeftSide = l.wsIdx === -1 || l.x1 < l.x2;
            const pathD = `M ${l.x1} ${l.y1} L ${l.x2} ${l.y2}`;
            const pathBack = `M ${l.x2} ${l.y2} L ${l.x1} ${l.y1}`;
            return (
              <g key={i}>
                <DashedLine
                  x1={l.x1}
                  y1={l.y1}
                  x2={l.x2}
                  y2={l.y2}
                  $toRight={!isLeftSide}
                />
                <TravelDot
                  r="3"
                  style={{ color: isLeftSide ? "#a78bfa" : "#818cf8" }}
                >
                  <animateMotion
                    path={pathD}
                    dur={`${1.8 + (i % 3) * 0.3}s`}
                    begin={`${(i % 5) * 0.4}s`}
                    repeatCount="indefinite"
                    calcMode="linear"
                  />
                </TravelDot>
                <TravelDot
                  r="2.5"
                  style={{ color: isLeftSide ? "#a78bfa" : "#818cf8" }}
                  opacity="0.5"
                >
                  <animateMotion
                    path={pathBack}
                    dur={`${2.2 + (i % 3) * 0.25}s`}
                    begin={`${0.8 + (i % 4) * 0.35}s`}
                    repeatCount="indefinite"
                    calcMode="linear"
                  />
                </TravelDot>
              </g>
            );
          })}
        </SvgOverlay>

        {/* ── Left: Agents ── */}
        <Column $align="flex-end">
          <ColumnLabel>Connected Agents</ColumnLabel>
          {agents.length === 0 ? (
            <EmptyColumn>No agents connected</EmptyColumn>
          ) : (
            agents.map((agent, i) => {
              const vis = agentVisual(agent.clientKind);
              const localPath = displayLocalPath(agent.localPath);
              const agentName = displayAgentName(agent);
              const methodLabel = agent.clientKind.trim() || "agent";
              const agentId = agent.agentId?.trim() ?? "";
              const copied = agentId !== "" && copiedAgentId === agentId;
              return (
                <AgentNode
                  key={agent.sessionId}
                  $i={i}
                  $canCopy={agentId !== ""}
                  $copied={copied}
                  aria-label={agentId ? `Copy agent ID ${agentId}` : "Agent ID not reported"}
                  title={agentId ? (copied ? "Copied agent ID" : "Copy agent ID") : "Agent ID not reported"}
                  onClick={() => {
                    void copyAgentId(agentId);
                  }}
                  ref={(el) => {
                    agentRefs.current[i] = el;
                  }}
                >
                  <StatusDot $active={agent.state === "active"} />
                  <AgentIcon $hue={vis.hue} title={methodLabel}>
                    {vis.icon}
                  </AgentIcon>
                  <AgentText>
                    <AgentLabel>{agentName}</AgentLabel>
                    <AgentMeta>
                      {agent.hostname || agent.sessionId.slice(0, 8)}
                    </AgentMeta>
                    {localPath ? (
                      <AgentPath title={localPath}>{localPath}</AgentPath>
                    ) : null}
                  </AgentText>
                </AgentNode>
              );
            })
          )}
        </Column>

        {/* ── Center: Redis Hub ── */}
        <HubWrap>
          <HubNode ref={hubRef}>
            <RedisLogoDarkMinIcon
              customSize="36px"
              style={{ filter: "brightness(0) invert(1)" }}
            />
            <HubLabel>Redis</HubLabel>
          </HubNode>
        </HubWrap>

        {/* ── Right: Workspaces ── */}
        <Column $align="flex-start">
          <ColumnLabel>Workspaces</ColumnLabel>
          {workspaces.length === 0 ? (
            <EmptyColumn>No workspaces yet</EmptyColumn>
          ) : (
            workspaces.map((ws, i) => (
              <WorkspaceNode
                key={ws.id}
                $i={i}
                ref={(el) => {
                  wsRefs.current[i] = el;
                }}
              >
                <FolderIcon>&#128193;</FolderIcon>
                <WorkspaceMeta>
                  <WorkspaceName>{ws.name}</WorkspaceName>
                  <WorkspaceFiles>
                    {ws.fileCount} file{ws.fileCount === 1 ? "" : "s"}
                  </WorkspaceFiles>
                </WorkspaceMeta>
              </WorkspaceNode>
            ))
          )}
        </Column>
      </Topology>
    </CardWrap>
  );
}
