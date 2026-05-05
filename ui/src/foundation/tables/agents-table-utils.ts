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
    return compareAgentValues(
      agentSortValue(left, sortBy),
      agentSortValue(right, sortBy),
      sortDirection,
    );
  });
}

function agentSortValue(agent: AFSAgentSession, field: AgentSortField): string {
  return agent[field]?.trim() ?? "";
}
