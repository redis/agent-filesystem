import type { AFSAgentSession } from "../types/afs";

export type AgentSortField =
  | "sessionName"
  | "agentName"
  | "hostname"
  | "workspaceName"
  | "localPath"
  | "lastSeenAt";

export function normalizeSearchValue(value?: string | null) {
  return value?.trim().toLowerCase() ?? "";
}

function trimmed(value?: string | null) {
  return value?.trim() ?? "";
}

export function displayAgentIdentityLabel(agent: AFSAgentSession): string {
  const agentName = trimmed(agent.agentName);
  const sessionName = trimmed(agent.sessionName);
  const machineName = trimmed(agent.hostname);

  if (agentName && sessionName) {
    return `${agentName} - ${sessionName}`;
  }

  if (sessionName) {
    return [sessionName, machineName].filter(Boolean).join(" - ");
  }

  if (agentName) {
    return [agentName, machineName].filter(Boolean).join(" - ");
  }

  return (
    machineName ||
    trimmed(agent.label) ||
    trimmed(agent.agentId) ||
    trimmed(agent.sessionId) ||
    "unknown agent"
  );
}

export function compareAgentValues(
  left: string | number,
  right: string | number,
  direction: "asc" | "desc",
) {
  const result =
    typeof left === "number" && typeof right === "number"
      ? left - right
      : String(left).localeCompare(String(right));

  return direction === "asc" ? result : result * -1;
}

export function matchesAgentSearch(agent: AFSAgentSession, query: string) {
  if (query === "") {
    return true;
  }

  return [
    displayAgentIdentityLabel(agent),
    agent.hostname,
    agent.agentId,
    agent.agentName,
    agent.sessionName,
    agent.localPath,
    agent.label,
    agent.workspaceName,
    agent.workspaceId,
    agent.databaseName,
  ]
    .map(normalizeSearchValue)
    .some((value) => value.includes(query));
}

export function filterAndSortAgents(
  rows: AFSAgentSession[],
  search: string,
  sortBy: AgentSortField,
  sortDirection: "asc" | "desc",
) {
  const query = normalizeSearchValue(search);
  const filteredRows = rows.filter((row) => matchesAgentSearch(row, query));

  return [...filteredRows].sort((left, right) => {
    const result = compareAgentValues(
      agentSortValue(left, sortBy),
      agentSortValue(right, sortBy),
      sortDirection,
    );
    if (result !== 0) {
      return result;
    }
    return compareAgentsByIdentity(left, right);
  });
}

export function compareAgentsByIdentity(
  left: AFSAgentSession,
  right: AFSAgentSession,
) {
  return agentStableSortValue(left).localeCompare(agentStableSortValue(right));
}

function agentSortValue(agent: AFSAgentSession, field: AgentSortField): string {
  if (field === "agentName") {
    return displayAgentIdentityLabel(agent);
  }
  return agent[field]?.trim() ?? "";
}

function agentStableSortValue(agent: AFSAgentSession): string {
  return [
    displayAgentIdentityLabel(agent),
    agent.workspaceName,
    agent.workspaceId,
    agent.sessionId,
  ]
    .map(normalizeSearchValue)
    .join("\u0000");
}
