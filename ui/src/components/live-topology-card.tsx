import { useMemo, useRef, useLayoutEffect, useState, useCallback } from "react";
import { useNavigate } from "@tanstack/react-router";
import styled, { keyframes } from "styled-components";
import { RedisLogoDarkMinIcon } from "@redis-ui/icons/multicolor";
import type { AFSAgentSession, AFSWorkspaceSummary } from "../foundation/types/afs";
import { BotIcon, FoldersIcon } from "./lucide-icons";
import { formatBytes } from "../foundation/api/afs";
import { AgentDetailDialog } from "../foundation/tables/agents-table";
import { displayWorkspaceName } from "../foundation/workspace-display";

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
const AgentNode = styled.button<{ $i: number }>`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel-strong);
  color: inherit;
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
  animation: ${fadeIn} 0.24s ease forwards;
  animation-delay: ${({ $i }) => $i * 0.06}s;
  opacity: 0;

  [data-theme="dark"] & {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:hover {
    border-color: var(--afs-accent, #dc2626);
    box-shadow: 0 4px 12px rgba(8, 6, 13, 0.08);
    transform: translateY(-1px);
  }

  [data-theme="dark"] &:hover {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 2px;
  }
`;

const NodeIconBox = styled.div<{ $active?: boolean }>`
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: ${({ $active }) => ($active ? "var(--afs-ok, #22c55e)" : "var(--afs-accent, #dc2626)")};
  background: color-mix(in srgb, currentColor 14%, transparent);
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
const WorkspaceNode = styled.button<{ $i: number }>`
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 10px;
  background: var(--afs-panel-strong);
  color: inherit;
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition:
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
  animation: ${fadeIn} 0.24s ease forwards;
  animation-delay: ${({ $i }) => 0.18 + $i * 0.06}s;
  opacity: 0;

  [data-theme="dark"] & {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:hover {
    border-color: var(--afs-accent, #dc2626);
    box-shadow: 0 4px 12px rgba(8, 6, 13, 0.08);
    transform: translateY(-1px);
  }

  [data-theme="dark"] &:hover {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 2px;
  }
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

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function displayLocalPath(path: string): string {
  return path.trim().replace(/^\/Users\/[^/]+\/?/, "~/");
}

function displaySystemName(agent: AFSAgentSession): string {
  return agent.hostname.trim() || "unknown host";
}

function displayAgentId(agent: AFSAgentSession): string {
  return (
    agent.agentId?.trim() ||
    agent.sessionId.trim() ||
    "id not reported"
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
  const navigate = useNavigate();
  const topologyRef = useRef<HTMLDivElement>(null);
  const agentRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const wsRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const hubRef = useRef<HTMLDivElement>(null);
  const animationFrameRef = useRef<number | null>(null);
  const [selectedAgent, setSelectedAgent] = useState<AFSAgentSession | null>(null);
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

  const activeAgents = agents.filter((a) => a.state === "active").length;

  return (
    <>
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
              const systemName = displaySystemName(agent);
              const agentId = displayAgentId(agent);
              const mountedPath = displayLocalPath(agent.localPath);
              const methodLabel = agent.clientKind.trim() || "agent";
              const active = agent.state === "active";
              return (
                <AgentNode
                  key={agent.sessionId}
                  $i={i}
                  type="button"
                  aria-label={`Open details for ${systemName}`}
                  title={`Open details for ${systemName}`}
                  onClick={() => {
                    setSelectedAgent(agent);
                  }}
                  ref={(el) => {
                    agentRefs.current[i] = el;
                  }}
                >
                  <NodeIconBox $active={active} title={methodLabel}>
                    <BotIcon customSize={18} />
                  </NodeIconBox>
                  <AgentText>
                    <AgentLabel title={systemName}>{systemName}</AgentLabel>
                    <AgentMeta title={agentId}>{agentId}</AgentMeta>
                    {mountedPath ? (
                      <AgentPath title={agent.localPath}>{mountedPath}</AgentPath>
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
            workspaces.map((ws, i) => {
              const workspaceLabel = displayWorkspaceName(ws.name);
              return (
                <WorkspaceNode
                  key={ws.id}
                  $i={i}
                  type="button"
                  aria-label={`Open workspace ${workspaceLabel}`}
                  title={`Open workspace ${workspaceLabel}`}
                  onClick={() => {
                    void navigate({
                      to: "/workspaces/$workspaceId",
                      params: { workspaceId: ws.id },
                      search: { databaseId: ws.databaseId },
                    });
                  }}
                  ref={(el) => {
                    wsRefs.current[i] = el;
                  }}
                >
                  <NodeIconBox title="Workspace">
                    <FoldersIcon customSize={18} />
                  </NodeIconBox>
                  <WorkspaceMeta>
                    <WorkspaceName>{workspaceLabel}</WorkspaceName>
                    <WorkspaceFiles>
                      {ws.fileCount} file{ws.fileCount === 1 ? "" : "s"}
                    </WorkspaceFiles>
                    <WorkspaceFiles>
                      Size: {formatBytes(ws.totalBytes)}
                    </WorkspaceFiles>
                  </WorkspaceMeta>
                </WorkspaceNode>
              );
            })
          )}
        </Column>
      </Topology>
    </CardWrap>
      {selectedAgent != null ? (
        <AgentDetailDialog
          agent={selectedAgent}
          onClose={() => setSelectedAgent(null)}
          onOpenWorkspace={(agent) => {
            setSelectedAgent(null);
            void navigate({
              to: "/workspaces/$workspaceId",
              params: { workspaceId: agent.workspaceId },
              search: agent.databaseId ? { databaseId: agent.databaseId } : {},
            });
          }}
        />
      ) : null}
    </>
  );
}
