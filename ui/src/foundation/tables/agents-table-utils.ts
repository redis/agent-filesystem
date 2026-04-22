import type { AFSAgentSession } from "../types/afs";

export type AgentSortField = "hostname" | "label" | "workspaceName" | "lastSeenAt";

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
    const leftValue =
      sortBy === "label"
        ? left.label ?? left.agentId ?? left.hostname
        : left[sortBy] ?? "";
    const rightValue =
      sortBy === "label"
        ? right.label ?? right.agentId ?? right.hostname
        : right[sortBy] ?? "";
    return compareAgentValues(leftValue, rightValue, sortDirection);
  });
}
