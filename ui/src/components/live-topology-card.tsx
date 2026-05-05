import { useMemo, useRef, useLayoutEffect, useState, useCallback, useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import styled, { css, keyframes } from "styled-components";
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

type NodePresence = "entering" | "present" | "exiting";

type AnimatedTopologyItem<T> = {
  id: string;
  item: T;
  presence: NodePresence;
};

type TopologyLine = {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  agentId: string;
  agentIdx: number;
  workspaceId: string;
  wsIdx: number;
  color: string;
};

type HoveredTopologyItem =
  | { kind: "agent"; id: string; workspaceId: string }
  | { kind: "workspace"; id: string };

const TOPOLOGY_NODE_EXIT_MS = 420;
const TOPOLOGY_MOTION_CONNECTION_LIMIT = 40;

const CONNECTION_COLORS = [
  "#a65f5f",
  "#5f78a6",
  "#638f70",
  "#a7825a",
  "#7b6aa6",
  "#5f8f99",
  "#a6658b",
  "#75865d",
];

const popIn = keyframes`
  0%   { opacity: 0; transform: scale(0.82) translateY(8px); }
  72%  { opacity: 1; transform: scale(1.035) translateY(0); }
  100% { opacity: 1; transform: scale(1) translateY(0); }
`;

const popOut = keyframes`
  0%   { opacity: 1; transform: scale(1) translateY(0); }
  100% { opacity: 0; transform: scale(0.8) translateY(-8px); }
`;

const marchLeft = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: -16; }
`;

const marchRight = keyframes`
  from { stroke-dashoffset: 0; }
  to   { stroke-dashoffset: 16; }
`;

const nodePresenceStyles = css<{ $i: number; $presence: NodePresence }>`
  ${({ $i, $presence }) =>
    $presence === "present"
      ? css`
          opacity: 1;
          transform: none;
        `
      : css`
          opacity: 0;
          pointer-events: ${$presence === "exiting" ? "none" : "auto"};
          animation: ${$presence === "exiting" ? popOut : popIn} ${TOPOLOGY_NODE_EXIT_MS}ms
            cubic-bezier(0.2, 0.8, 0.2, 1) forwards;
          animation-delay: ${$presence === "entering" ? `${Math.min($i, 6) * 0.04}s` : "0s"};
        `}
`;

/* ---- Outer card ---- */
const CardWrap = styled.div`
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 16px;
  background: var(--afs-panel-strong);
  padding: 24px;
  overflow: hidden;
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
  --topology-node-min: 180px;
  --topology-node-max: 240px;
  --topology-hub-size: 80px;

  display: grid;
  grid-template-columns:
    fit-content(var(--topology-node-max))
    minmax(var(--topology-hub-size), 1fr)
    fit-content(var(--topology-node-max));
  align-items: stretch;
  gap: 0;
  min-height: 160px;
  position: relative;

  @media (max-width: 720px) {
    grid-template-columns: 1fr;
    gap: 16px;
  }
`;

/* ---- Column wrappers ---- */
const Column = styled.div<{ $align?: string; $justify?: string }>`
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: ${({ $align }) => $align ?? "stretch"};
  justify-content: ${({ $justify }) => $justify ?? "flex-start"};
  width: 100%;
  min-height: 0;
  z-index: 1;
`;

const ColumnLabel = styled.div<{ $align?: "left" | "right" | "center" }>`
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--afs-muted, #71717a);
  margin-bottom: 2px;
  text-align: ${({ $align }) => $align ?? "center"};
`;

/* ---- Agent nodes ---- */
const AgentNode = styled.button<{ $i: number; $presence: NodePresence; $highlighted?: boolean }>`
  display: flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  min-width: var(--topology-node-min);
  max-width: var(--topology-node-max);
  box-sizing: border-box;
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
  ${nodePresenceStyles}

  [data-theme="dark"] & {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:hover {
    border-color: var(--afs-accent, #dc2626);
    box-shadow: 0 4px 12px rgba(8, 6, 13, 0.08);
    transform: translateY(-1px);
  }

  ${({ $highlighted }) =>
    $highlighted
      ? css`
          border-color: var(--afs-accent, #dc2626);
          background: color-mix(in srgb, var(--afs-accent, #dc2626) 8%, var(--afs-panel-strong));
          box-shadow: 0 6px 18px rgba(8, 6, 13, 0.12);
          transform: translateY(-1px);
        `
      : null}

  [data-theme="dark"] &:hover,
  [data-theme="dark"] &[data-highlighted="true"] {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 2px;
  }

  &:disabled {
    cursor: default;
  }

  @media (max-width: 720px) {
    width: 100%;
    max-width: none;
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
  display: block;
  font-size: 12px;
  font-weight: 800;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
`;

const AgentText = styled.div`
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-width: 0;
  max-width: 100%;
`;

const AgentMeta = styled.span`
  display: block;
  font-size: 10px;
  color: var(--afs-muted, #71717a);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
`;

const AgentPath = styled.span`
  display: block;
  max-width: 100%;
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
const WorkspaceNode = styled.button<{ $i: number; $presence: NodePresence; $highlighted?: boolean }>`
  display: flex;
  align-items: center;
  gap: 8px;
  width: fit-content;
  min-width: var(--topology-node-min);
  max-width: var(--topology-node-max);
  box-sizing: border-box;
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
  ${nodePresenceStyles}

  [data-theme="dark"] & {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:hover {
    border-color: var(--afs-accent, #dc2626);
    box-shadow: 0 4px 12px rgba(8, 6, 13, 0.08);
    transform: translateY(-1px);
  }

  ${({ $highlighted }) =>
    $highlighted
      ? css`
          border-color: var(--afs-accent, #dc2626);
          background: color-mix(in srgb, var(--afs-accent, #dc2626) 8%, var(--afs-panel-strong));
          box-shadow: 0 6px 18px rgba(8, 6, 13, 0.12);
          transform: translateY(-1px);
        `
      : null}

  [data-theme="dark"] &:hover,
  [data-theme="dark"] &[data-highlighted="true"] {
    border-color: var(--afs-ok, #dcff1e);
  }

  &:focus-visible {
    outline: 2px solid var(--afs-accent, #dc2626);
    outline-offset: 2px;
  }

  &:disabled {
    cursor: default;
  }

  @media (max-width: 720px) {
    width: 100%;
    max-width: none;
  }
`;

const WorkspaceMeta = styled.div`
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-width: 0;
  max-width: 100%;
`;

const WorkspaceName = styled.span`
  display: block;
  font-size: 12px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
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
  overflow: hidden;
  z-index: 0;

  @media (max-width: 720px) {
    display: none;
  }
`;

const DashedLine = styled.line<{ $toRight?: boolean; $color: string; $highlighted?: boolean; $animated: boolean }>`
  stroke: ${({ $color }) => $color};
  color: ${({ $color }) => $color};
  stroke-width: ${({ $highlighted }) => ($highlighted ? 3 : 2)};
  stroke-dasharray: 4 4;
  opacity: ${({ $highlighted }) => ($highlighted ? 0.95 : 0.55)};
  filter: ${({ $highlighted }) => ($highlighted ? "drop-shadow(0 0 4px currentColor)" : "none")};
  animation: ${({ $animated, $toRight }) =>
    $animated
      ? css`
          ${$toRight ? marchRight : marchLeft} 1s linear infinite
        `
      : "none"};
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
  width: 100%;
  box-sizing: border-box;
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

function displayAgentPrimaryName(agent: AFSAgentSession): string {
  const sessionName = agent.sessionName?.trim();
  if (sessionName) {
    return sessionName;
  }
  return (
    agent.agentName?.trim() ||
    agent.label?.trim() ||
    agent.hostname.trim() ||
    agent.agentId?.trim() ||
    agent.sessionId.trim() ||
    "unknown agent"
  );
}

function displayAgentMeta(agent: AFSAgentSession, primaryName: string): string {
  const sessionName = agent.sessionName?.trim();
  const agentName = agent.agentName?.trim();
  const hostname = agent.hostname.trim();
  const agentId = agent.agentId?.trim();
  const parts: string[] = [];
  if (sessionName && agentName) {
    parts.push(agentName);
  } else if (sessionName && !agentName && agentId) {
    parts.push(agentId);
  } else if (agentName && agentName !== primaryName) {
    parts.push(agentName);
  }
  if (hostname && hostname !== primaryName && !parts.includes(hostname)) {
    parts.push(hostname);
  }
  if (agentId && agentId !== primaryName && !parts.includes(agentId)) {
    parts.push(agentId);
  }
  return parts.join(" · ") || agent.sessionId.trim() || "id not reported";
}

function getAgentTopologyId(agent: AFSAgentSession): string {
  return agent.sessionId;
}

function getWorkspaceTopologyId(workspace: AFSWorkspaceSummary): string {
  return workspace.id;
}

function connectionColor(agentId: string, workspaceId: string): string {
  const key = `${agentId}:${workspaceId}`;
  let hash = 0;
  for (let i = 0; i < key.length; i += 1) {
    hash = (hash * 31 + key.charCodeAt(i)) >>> 0;
  }
  return CONNECTION_COLORS[hash % CONNECTION_COLORS.length];
}

function sortWorkspacesForTopology(
  agents: AFSAgentSession[],
  workspaces: AFSWorkspaceSummary[],
): AFSWorkspaceSummary[] {
  const workspaceById = new Map(workspaces.map((workspace) => [workspace.id, workspace]));
  const connectedIds: string[] = [];
  const seen = new Set<string>();

  agents.forEach((agent) => {
    if (workspaceById.has(agent.workspaceId) && !seen.has(agent.workspaceId)) {
      seen.add(agent.workspaceId);
      connectedIds.push(agent.workspaceId);
    }
  });

  const connected = connectedIds
    .map((id) => workspaceById.get(id))
    .filter((workspace): workspace is AFSWorkspaceSummary => workspace != null);
  const disconnected = workspaces.filter((workspace) => !seen.has(workspace.id));

  return [...connected, ...disconnected];
}

function sortAgentsForTopology(
  agents: AFSAgentSession[],
  sortedWorkspaces: AFSWorkspaceSummary[],
): AFSAgentSession[] {
  const workspaceRank = new Map(sortedWorkspaces.map((workspace, index) => [workspace.id, index]));

  return agents
    .map((agent, index) => ({ agent, index }))
    .sort((a, b) => {
      const aRank = workspaceRank.get(a.agent.workspaceId) ?? Number.MAX_SAFE_INTEGER;
      const bRank = workspaceRank.get(b.agent.workspaceId) ?? Number.MAX_SAFE_INTEGER;
      if (aRank !== bRank) return aRank - bRank;
      return a.index - b.index;
    })
    .map(({ agent }) => agent);
}

function agentIsHighlighted(agent: AFSAgentSession, hovered: HoveredTopologyItem | null): boolean {
  if (hovered == null) return false;
  if (hovered.kind === "agent") return hovered.id === agent.sessionId;
  return hovered.id === agent.workspaceId;
}

function workspaceIsHighlighted(workspace: AFSWorkspaceSummary, hovered: HoveredTopologyItem | null): boolean {
  if (hovered == null) return false;
  if (hovered.kind === "workspace") return hovered.id === workspace.id;
  return hovered.workspaceId === workspace.id;
}

function lineIsHighlighted(line: TopologyLine, hovered: HoveredTopologyItem | null): boolean {
  if (hovered == null) return false;
  if (hovered.kind === "agent") return hovered.id === line.agentId;
  return hovered.id === line.workspaceId;
}

function isVisibleTopologyItem<T>(
  row: AnimatedTopologyItem<T>,
): row is AnimatedTopologyItem<T> & { presence: Exclude<NodePresence, "exiting"> } {
  return row.presence !== "exiting";
}

function useAnimatedTopologyItems<T>(
  items: T[],
  getId: (item: T) => string,
): AnimatedTopologyItem<T>[] {
  const [rendered, setRendered] = useState<AnimatedTopologyItem<T>[]>(() =>
    items.map((item) => ({ id: getId(item), item, presence: "entering" })),
  );

  useEffect(() => {
    const incomingIds = new Set(items.map(getId));
    const incomingById = new Map(items.map((item) => [getId(item), item]));

    setRendered((previous) => {
      const previousById = new Map(previous.map((row) => [row.id, row]));
      const next = items.map((item) => {
        const id = getId(item);
        const previousRow = previousById.get(id);
        return {
          id,
          item,
          presence:
            previousRow == null || previousRow.presence === "exiting"
              ? "entering"
              : previousRow.presence,
        };
      });

      previous.forEach((row) => {
        if (!incomingIds.has(row.id) && row.presence !== "exiting") {
          next.push({ ...row, presence: "exiting" });
        }
      });

      return next;
    });

    const enterTimer = window.setTimeout(() => {
      setRendered((current) =>
        current.map((row) =>
          incomingById.has(row.id) && row.presence === "entering"
            ? { ...row, presence: "present" }
            : row,
        ),
      );
    }, TOPOLOGY_NODE_EXIT_MS);

    const exitTimer = window.setTimeout(() => {
      setRendered((current) =>
        current.filter((row) => incomingIds.has(row.id) || row.presence !== "exiting"),
      );
    }, TOPOLOGY_NODE_EXIT_MS);

    return () => {
      window.clearTimeout(enterTimer);
      window.clearTimeout(exitTimer);
    };
  }, [items, getId]);

  return rendered;
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
  const sortedWorkspaces = useMemo(
    () => sortWorkspacesForTopology(agents, workspaces),
    [agents, workspaces],
  );
  const sortedAgents = useMemo(
    () => sortAgentsForTopology(agents, sortedWorkspaces),
    [agents, sortedWorkspaces],
  );
  const animatedAgents = useAnimatedTopologyItems(sortedAgents, getAgentTopologyId);
  const animatedWorkspaces = useAnimatedTopologyItems(sortedWorkspaces, getWorkspaceTopologyId);
  const visibleAgents = useMemo(
    () => animatedAgents.filter(isVisibleTopologyItem),
    [animatedAgents],
  );
  const visibleWorkspaces = useMemo(
    () => animatedWorkspaces.filter(isVisibleTopologyItem),
    [animatedWorkspaces],
  );
  const topologyRef = useRef<HTMLDivElement>(null);
  const agentRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const wsRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const hubRef = useRef<HTMLDivElement>(null);
  const animationFrameRef = useRef<number | null>(null);
  const [selectedAgent, setSelectedAgent] = useState<AFSAgentSession | null>(null);
  const [lines, setLines] = useState<TopologyLine[]>([]);

  // Build a map: workspaceId -> index in the displayed workspace list
  const wsIndexMap = useMemo(() => {
    const map = new Map<string, number>();
    visibleWorkspaces.forEach(({ item }, i) => map.set(item.id, i));
    return map;
  }, [visibleWorkspaces]);

  // Build connection pairs: [agentIdx, wsIdx]
  const connections = useMemo(() => {
    const pairs: { agentId: string; agentIdx: number; workspaceId: string; wsIdx: number; color: string }[] = [];
    const seen = new Set<string>();
    visibleAgents.forEach(({ item: agent }, aIdx) => {
      const wIdx = wsIndexMap.get(agent.workspaceId);
      if (wIdx != null) {
        const key = `${aIdx}-${wIdx}`;
        if (!seen.has(key)) {
          seen.add(key);
          pairs.push({
            agentId: agent.sessionId,
            agentIdx: aIdx,
            workspaceId: agent.workspaceId,
            wsIdx: wIdx,
            color: connectionColor(agent.sessionId, agent.workspaceId),
          });
        }
      }
    });
    return pairs;
  }, [visibleAgents, wsIndexMap]);

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

    const newLines: TopologyLine[] = [];

    // Draw only explicit agent -> workspace connections through the hub.
    connections.forEach(({ agentId, agentIdx, workspaceId, wsIdx, color }) => {
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
        agentId,
        agentIdx,
        workspaceId,
        wsIdx,
        color,
      });
      newLines.push({
        x1: hubCx,
        y1: hubCy,
        x2: wRect.left - cRect.left,
        y2: wRect.top + wRect.height / 2 - cRect.top,
        agentId,
        agentIdx,
        workspaceId,
        wsIdx,
        color,
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
      ...agentRefs.current.slice(0, visibleAgents.length),
      ...wsRefs.current.slice(0, visibleWorkspaces.length),
    ].filter((element): element is HTMLElement => element != null);

    observedElements.forEach((element) => resizeObserver?.observe(element));

    computeLines();
    window.addEventListener("resize", scheduleLineCompute);

    return () => {
      window.removeEventListener("resize", scheduleLineCompute);
      resizeObserver?.disconnect();
      if (animationFrameRef.current != null) {
        cancelAnimationFrame(animationFrameRef.current);
        animationFrameRef.current = null;
      }
    };
  }, [computeLines, visibleAgents.length, visibleWorkspaces.length, scheduleLineCompute]);

  const activeAgents = agents.filter((a) => a.state === "active").length;
  const [hoveredItem, setHoveredItem] = useState<HoveredTopologyItem | null>(null);
  const animateConnectionMotion = connections.length <= TOPOLOGY_MOTION_CONNECTION_LIMIT;

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
            const travelsRight = l.x1 < l.x2;
            const pathD = `M ${l.x1} ${l.y1} L ${l.x2} ${l.y2}`;
            const pathBack = `M ${l.x2} ${l.y2} L ${l.x1} ${l.y1}`;
            const highlighted = lineIsHighlighted(l, hoveredItem);
            return (
              <g key={i}>
                <DashedLine
                  x1={l.x1}
                  y1={l.y1}
                  x2={l.x2}
                  y2={l.y2}
                  $toRight={travelsRight}
                  $color={l.color}
                  $highlighted={highlighted}
                  $animated={animateConnectionMotion}
                />
                {animateConnectionMotion ? (
                  <>
                    <TravelDot
                      r="3"
                      style={{ color: l.color }}
                      opacity={highlighted ? "1" : "0.72"}
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
                      style={{ color: l.color }}
                      opacity={highlighted ? "0.82" : "0.5"}
                    >
                      <animateMotion
                        path={pathBack}
                        dur={`${2.2 + (i % 3) * 0.25}s`}
                        begin={`${0.8 + (i % 4) * 0.35}s`}
                        repeatCount="indefinite"
                        calcMode="linear"
                      />
                    </TravelDot>
                  </>
                ) : null}
              </g>
            );
          })}
        </SvgOverlay>

        {/* ── Left: Agents ── */}
        <Column $align="stretch" $justify="center">
          <ColumnLabel $align="left">Connected Agents</ColumnLabel>
          {visibleAgents.length === 0 ? (
            <EmptyColumn>No agents connected</EmptyColumn>
          ) : (
            visibleAgents.map(({ item: agent, presence }, i) => {
              const agentName = displayAgentPrimaryName(agent);
              const agentMeta = displayAgentMeta(agent, agentName);
              const mountedPath = displayLocalPath(agent.localPath);
              const methodLabel = agent.clientKind.trim() || "agent";
              const active = agent.state === "active";
              const highlighted = agentIsHighlighted(agent, hoveredItem);
              return (
                <AgentNode
                  key={agent.sessionId}
                  $i={i}
                  $presence={presence}
                  $highlighted={highlighted}
                  data-highlighted={highlighted}
                  type="button"
                  aria-label={`Open details for ${agentName}`}
                  title={`Open details for ${agentName}`}
                  onMouseEnter={() => {
                    setHoveredItem({
                      kind: "agent",
                      id: agent.sessionId,
                      workspaceId: agent.workspaceId,
                    });
                  }}
                  onMouseLeave={() => {
                    setHoveredItem(null);
                  }}
                  onFocus={() => {
                    setHoveredItem({
                      kind: "agent",
                      id: agent.sessionId,
                      workspaceId: agent.workspaceId,
                    });
                  }}
                  onBlur={() => {
                    setHoveredItem(null);
                  }}
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
                    <AgentLabel title={agentName}>{agentName}</AgentLabel>
                    <AgentMeta title={agentMeta}>{agentMeta}</AgentMeta>
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
        <Column $align="stretch" $justify="center">
          <ColumnLabel $align="right">Workspaces</ColumnLabel>
          {visibleWorkspaces.length === 0 ? (
            <EmptyColumn>No workspaces yet</EmptyColumn>
          ) : (
            visibleWorkspaces.map(({ item: ws, presence }, i) => {
              const workspaceLabel = displayWorkspaceName(ws.name);
              const highlighted = workspaceIsHighlighted(ws, hoveredItem);
              return (
                <WorkspaceNode
                  key={ws.id}
                  $i={i}
                  $presence={presence}
                  $highlighted={highlighted}
                  data-highlighted={highlighted}
                  type="button"
                  aria-label={`Open workspace ${workspaceLabel}`}
                  title={`Open workspace ${workspaceLabel}`}
                  onMouseEnter={() => {
                    setHoveredItem({ kind: "workspace", id: ws.id });
                  }}
                  onMouseLeave={() => {
                    setHoveredItem(null);
                  }}
                  onFocus={() => {
                    setHoveredItem({ kind: "workspace", id: ws.id });
                  }}
                  onBlur={() => {
                    setHoveredItem(null);
                  }}
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
