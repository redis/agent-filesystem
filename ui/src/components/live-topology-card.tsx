import { useMemo, useRef, useLayoutEffect, useState, useCallback, useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";
import styled, { css, keyframes } from "styled-components";
import { RedisLogoDarkMinIcon } from "@redis-ui/icons/multicolor";
import type { AFSAgentSession, AFSWorkspaceSummary } from "../foundation/types/afs";
import { BotIcon, DatabaseIcon, FoldersIcon, LaptopIcon } from "./lucide-icons";
import { formatBytes } from "../foundation/api/afs";
import { AgentDetailDialog } from "../foundation/tables/agents-table";
import {
  compareAgentsByIdentity,
  displayAgentIdentityLabel,
} from "../foundation/tables/agents-table-utils";
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
  // Polyline points: 3 points forming a stub-elbow-hub shape.
  // For agent-side segments: [agentEdge, bend, hub]
  // For ws-side segments:    [hub, bend, wsEdge]
  points: [number, number][];
  // The endpoint that touches an agent or workspace box (gets a static dot).
  // Hub end has no dot.
  endpointDot?: [number, number];
  isAgentSide: boolean;
  agentId: string;
  agentIdx: number;
  workspaceId: string;
  wsIdx: number;
  color: string;
};

const TOPOLOGY_LINE_STUB = 24;

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
  font-size: 18px;
  font-weight: 700;
  color: var(--afs-ink, #18181b);
  letter-spacing: -0.01em;
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
  flex-direction: column;
  align-items: center;
  justify-content: center;
  align-self: center;
  gap: 10px;
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
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.02em;
  opacity: 0.95;
`;

const HubCaption = styled.div`
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  text-align: center;
  max-width: 160px;
`;

const HubCaptionValue = styled.span`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 11px;
  font-weight: 600;
  color: var(--afs-ink, #18181b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
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
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
`;

/* ---- Host group (left column) ---- */
const HostGroup = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 12px;
  background: color-mix(in srgb, var(--afs-panel-strong) 80%, transparent);

  [data-theme="dark"] & {
    border-color: color-mix(in srgb, var(--afs-ok, #dcff1e) 45%, transparent);
  }

  @media (max-width: 720px) {
    padding: 8px;
  }
`;

const HostGroupHeader = styled.div`
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px;
  color: var(--afs-ink, #18181b);

  [data-theme="dark"] & {
    color: var(--afs-ok, #dcff1e);
  }
`;

const HostGroupName = styled.span`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`;

/* ---- Database group ---- */
const DatabaseGroup = styled.div`
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--afs-line, #e4e4e7);
  border-radius: 12px;
  background: color-mix(in srgb, var(--afs-panel-strong) 80%, transparent);

  [data-theme="dark"] & {
    border-color: color-mix(in srgb, var(--afs-ok, #dcff1e) 45%, transparent);
  }

  @media (max-width: 720px) {
    padding: 8px;
  }
`;

const DatabaseGroupHeader = styled.div`
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 4px;
  color: var(--afs-ink, #18181b);

  [data-theme="dark"] & {
    color: var(--afs-ok, #dcff1e);
  }
`;

const DatabaseGroupName = styled.span`
  font-family: var(--afs-mono, ui-monospace, SFMono-Regular, Menlo, monospace);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
`;

/* ---- Live status footer pill ---- */
const LiveStatusFooter = styled.div`
  display: flex;
  justify-content: center;
  margin-top: 24px;
`;

const LiveStatusPill = styled.div`
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px;
  border-radius: 999px;
  border: 1px solid var(--afs-line, #e4e4e7);
  background: var(--afs-panel-strong);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--afs-muted, #71717a);
`;

const LiveStatusDot = styled.span`
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--afs-ok, #22c55e);
  box-shadow: 0 0 8px color-mix(in srgb, var(--afs-ok, #22c55e) 70%, transparent);
  flex-shrink: 0;
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

const DashedPolyline = styled.polyline<{ $color: string; $highlighted?: boolean; $animated: boolean }>`
  stroke: ${({ $color }) => $color};
  color: ${({ $color }) => $color};
  stroke-width: ${({ $highlighted }) => ($highlighted ? 3 : 2)};
  stroke-linecap: round;
  stroke-linejoin: round;
  fill: none;
  stroke-dasharray: 4 4;
  opacity: ${({ $highlighted }) => ($highlighted ? 0.95 : 0.55)};
  filter: ${({ $highlighted }) => ($highlighted ? "drop-shadow(0 0 4px currentColor)" : "none")};
  animation: ${({ $animated }) =>
    $animated
      ? css`
          ${marchRight} 1s linear infinite
        `
      : "none"};
`;

const EndpointDot = styled.circle<{ $highlighted?: boolean }>`
  fill: currentColor;
  stroke: var(--afs-panel-strong);
  stroke-width: 1.5;
  opacity: ${({ $highlighted }) => ($highlighted ? 1 : 0.85)};
  filter: ${({ $highlighted }) => ($highlighted ? "drop-shadow(0 0 4px currentColor)" : "none")};
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

// Topology-specific label: the parent host group already shows the host, and
// `agentName` often duplicates it as a friendly prefix (e.g. "Macbook Air").
// Prefer the session name, then the agent name; never return the hostname.
// When neither is set, fall back to a short session id ("Session: a1b2c3d4").
function displayAgentPrimaryName(agent: AFSAgentSession): string {
  const host = agent.hostname?.trim() ?? "";
  const notHost = (value?: string | null) => {
    const trimmed = value?.trim();
    return trimmed && trimmed !== host ? trimmed : undefined;
  };
  const named = notHost(agent.sessionName) || notHost(agent.agentName);
  if (named) return named;
  const shortId = agent.sessionId?.trim().slice(0, 8);
  return shortId ? `Session: ${shortId}` : "Session: unknown";
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

function sortAgentsForTopology(agents: AFSAgentSession[]): AFSAgentSession[] {
  return [...agents].sort(compareAgentsByIdentity);
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
  const sortedAgents = useMemo(
    () => sortAgentsForTopology(agents),
    [agents],
  );
  const sortedWorkspaces = useMemo(
    () => sortWorkspacesForTopology(sortedAgents, workspaces),
    [sortedAgents, workspaces],
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
    // Each connection becomes two stub-elbow polylines: agent -> hub, hub -> ws.
    connections.forEach(({ agentId, agentIdx, workspaceId, wsIdx, color }) => {
      const aEl = agentRefs.current[agentIdx];
      const wEl = wsRefs.current[wsIdx];
      if (!aEl || !wEl) return;

      const aRect = aEl.getBoundingClientRect();
      const wRect = wEl.getBoundingClientRect();
      const ax = aRect.right - cRect.left;
      const ay = aRect.top + aRect.height / 2 - cRect.top;
      const wx = wRect.left - cRect.left;
      const wy = wRect.top + wRect.height / 2 - cRect.top;
      const aBend = Math.min(ax + TOPOLOGY_LINE_STUB, hubCx);
      const wBend = Math.max(wx - TOPOLOGY_LINE_STUB, hubCx);

      newLines.push({
        points: [
          [ax, ay],
          [aBend, ay],
          [hubCx, hubCy],
        ],
        endpointDot: [ax, ay],
        isAgentSide: true,
        agentId,
        agentIdx,
        workspaceId,
        wsIdx,
        color,
      });
      newLines.push({
        points: [
          [hubCx, hubCy],
          [wBend, wy],
          [wx, wy],
        ],
        endpointDot: [wx, wy],
        isAgentSide: false,
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

  // Group visible agents by hostname so each "computer" is a single box.
  // The flat visibleIndex is preserved so agentRefs stays in sync with
  // connection-line indexing.
  const agentGroups = useMemo(() => {
    type Row = {
      agent: AFSAgentSession;
      presence: NodePresence;
      visibleIndex: number;
    };
    const groups = new Map<string, { hostname: string; rows: Row[] }>();
    visibleAgents.forEach(({ item: agent, presence }, visibleIndex) => {
      const key = agent.hostname.trim() || "Unknown host";
      let group = groups.get(key);
      if (!group) {
        group = { hostname: key, rows: [] };
        groups.set(key, group);
      }
      group.rows.push({ agent, presence, visibleIndex });
    });
    return Array.from(groups.values());
  }, [visibleAgents]);

  // Group visible workspaces by database, preserving the flat visible index
  // so wsRefs stays in sync with the connection-line indexing.
  const workspaceGroups = useMemo(() => {
    type Row = {
      ws: AFSWorkspaceSummary;
      presence: NodePresence;
      visibleIndex: number;
    };
    const groups = new Map<
      string,
      { databaseId: string; databaseName: string; rows: Row[] }
    >();
    visibleWorkspaces.forEach(({ item: ws, presence }, visibleIndex) => {
      const key = ws.databaseId || "_no_database";
      let group = groups.get(key);
      if (!group) {
        group = {
          databaseId: ws.databaseId,
          databaseName: ws.databaseName?.trim() || "Unassigned database",
          rows: [],
        };
        groups.set(key, group);
      }
      group.rows.push({ ws, presence, visibleIndex });
    });
    return Array.from(groups.values());
  }, [visibleWorkspaces]);

  // The hub represents the AFS control-plane (this server). Show the host
  // the browser is connected to — that's the ground truth for where agents
  // and workspaces are routed through.
  const hubEndpointCaption = useMemo(() => {
    if (typeof window === "undefined") return null;
    const host = window.location.host || "";
    return host ? { value: host } : null;
  }, []);

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
            const pointsAttr = l.points.map(([x, y]) => `${x},${y}`).join(" ");
            const pathD = l.points
              .map(([x, y], idx) => `${idx === 0 ? "M" : "L"} ${x} ${y}`)
              .join(" ");
            const pathBack = [...l.points]
              .reverse()
              .map(([x, y], idx) => `${idx === 0 ? "M" : "L"} ${x} ${y}`)
              .join(" ");
            const highlighted = lineIsHighlighted(l, hoveredItem);
            return (
              <g key={i}>
                <DashedPolyline
                  points={pointsAttr}
                  $color={l.color}
                  $highlighted={highlighted}
                  $animated={animateConnectionMotion}
                />
                {l.endpointDot ? (
                  <EndpointDot
                    cx={l.endpointDot[0]}
                    cy={l.endpointDot[1]}
                    r={4}
                    style={{ color: l.color }}
                    $highlighted={highlighted}
                  />
                ) : null}
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

        {/* ── Left: Agents grouped by host ── */}
        <Column $align="stretch" $justify="center">
          <ColumnLabel $align="left">Hosts / Agents</ColumnLabel>
          {visibleAgents.length === 0 ? (
            <EmptyColumn>No agents connected</EmptyColumn>
          ) : (
            agentGroups.map((group) => (
              <HostGroup key={group.hostname}>
                <HostGroupHeader>
                  <LaptopIcon customSize={14} />
                  <HostGroupName title={group.hostname}>
                    {group.hostname}
                  </HostGroupName>
                </HostGroupHeader>
                {group.rows.map(({ agent, presence, visibleIndex: i }) => {
                  const agentName = displayAgentPrimaryName(agent);
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
                        {mountedPath ? (
                          <AgentPath title={agent.localPath}>{mountedPath}</AgentPath>
                        ) : null}
                      </AgentText>
                    </AgentNode>
                  );
                })}
              </HostGroup>
            ))
          )}
        </Column>

        {/* ── Center: Redis Hub ── */}
        <HubWrap>
          <HubNode ref={hubRef}>
            <RedisLogoDarkMinIcon
              customSize="36px"
              style={{ filter: "brightness(0) invert(1)" }}
            />
            <HubLabel>control-plane</HubLabel>
          </HubNode>
          {hubEndpointCaption ? (
            <HubCaption>
              <HubCaptionValue title={hubEndpointCaption.value}>
                {hubEndpointCaption.value}
              </HubCaptionValue>
            </HubCaption>
          ) : null}
        </HubWrap>

        {/* ── Right: Databases / Workspaces ── */}
        <Column $align="stretch" $justify="center">
          <ColumnLabel $align="right">Databases / Workspaces</ColumnLabel>
          {visibleWorkspaces.length === 0 ? (
            <EmptyColumn>No workspaces yet</EmptyColumn>
          ) : (
            workspaceGroups.map((group) => (
              <DatabaseGroup key={group.databaseId || "_no_database"}>
                <DatabaseGroupHeader>
                  <DatabaseIcon customSize={14} />
                  <DatabaseGroupName title={group.databaseName}>
                    {group.databaseName}
                  </DatabaseGroupName>
                </DatabaseGroupHeader>
                {group.rows.map(({ ws, presence, visibleIndex: i }) => {
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
                          {ws.fileCount} file{ws.fileCount === 1 ? "" : "s"} ·
                          {" "}Size: {formatBytes(ws.totalBytes)}
                        </WorkspaceFiles>
                      </WorkspaceMeta>
                    </WorkspaceNode>
                  );
                })}
              </DatabaseGroup>
            ))
          )}
        </Column>
      </Topology>

      <LiveStatusFooter>
        <LiveStatusPill role="status" aria-live="polite">
          <span>live</span>
          <LiveStatusDot aria-hidden="true" />
        </LiveStatusPill>
      </LiveStatusFooter>
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
