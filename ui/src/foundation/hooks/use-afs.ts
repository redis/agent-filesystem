import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { afsApi, getAFSClientMode, monitorStreamURL } from "../api/afs";
import type { ListActivityInput, ListChangelogInput, ListEventsInput } from "../api/afs";
import type {
  AFSAgentSession,
  CreateSavepointInput,
  CreateWorkspaceInput,
  DiffFileVersionsInput,
  GetFileHistoryInput,
  GetFileVersionContentInput,
  GetWorkspaceFileContentInput,
  GetWorkspaceDiffInput,
  GetWorkspaceTreeInput,
  GetWorkspaceVersioningPolicyInput,
  RestoreSavepointInput,
  RestoreFileVersionInput,
  SaveDatabaseInput,
  UndeleteFileVersionInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
  UpdateWorkspaceVersioningPolicyInput,
  QuickstartInput,
  ImportLocalInput,
  CreateMCPTokenInput,
  CreateControlPlaneTokenInput,
} from "../types/afs";

const LIVE_QUERY_STALE_MS = 10_000;
const LIVE_QUERY_GC_MS = 10 * 60 * 1000;
const AGENT_QUERY_STALE_MS = 5_000;
const AGENT_QUERY_GC_MS = 5 * 60 * 1000;
const FILESYSTEM_QUERY_STALE_MS = 30_000;
const FILESYSTEM_QUERY_GC_MS = 5 * 60 * 1000;

type InfiniteChangelogInput = Omit<ListChangelogInput, "since" | "until">;

type MonitorStreamEvent = {
  type?: string;
  reason?: string;
};

const MONITOR_INVALIDATION_DEBOUNCE_MS = 250;

export const afsKeys = {
  all: ["afs"] as const,
  account: () => [...afsKeys.all, "account"] as const,
  adminOverview: () => [...afsKeys.all, "admin", "overview"] as const,
  adminUsers: () => [...afsKeys.all, "admin", "users"] as const,
  adminDatabases: () => [...afsKeys.all, "admin", "databases"] as const,
  adminWorkspaceSummaries: () => [...afsKeys.all, "admin", "workspaces"] as const,
  adminAgents: () => [...afsKeys.all, "admin", "agents"] as const,
  databases: () => [...afsKeys.all, "databases"] as const,
  workspaceSummaries: (databaseId: string | null) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", "summaries"] as const,
  workspace: (databaseId: string | null, workspaceId: string) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", workspaceId] as const,
  agents: (databaseId: string | null) =>
    [...afsKeys.all, "agents", databaseId ?? "all"] as const,
  activity: (input: ListActivityInput) =>
    [
      ...afsKeys.all,
      "activity",
      input.databaseId ?? "all",
      input.workspaceId ?? "all",
      input.limit ?? 50,
      input.until ?? "",
    ] as const,
  events: (input: ListEventsInput) =>
    [
      ...afsKeys.all,
      "events",
      input.databaseId ?? "all",
      input.workspaceId ?? "all",
      input.kind ?? "all",
      input.sessionId ?? "all",
      input.path ?? "",
      input.limit ?? 100,
      input.direction ?? "desc",
      input.since ?? "",
      input.until ?? "",
    ] as const,
  changelog: (input: ListChangelogInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId ?? "all",
      "changes",
      input.sessionId ?? "all",
      input.path ?? "",
      input.limit ?? 100,
      input.direction ?? "desc",
      input.since ?? "",
      input.until ?? "",
    ] as const,
  changelogFeed: (input: InfiniteChangelogInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId ?? "all",
      "changes-feed",
      input.sessionId ?? "all",
      input.path ?? "",
      input.limit ?? 100,
      input.direction ?? "desc",
    ] as const,
  workspaceTree: (input: GetWorkspaceTreeInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "tree",
      input.view,
      input.path,
      input.depth ?? 1,
    ] as const,
  workspaceFile: (input: GetWorkspaceFileContentInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "files",
      input.view,
      input.path,
    ] as const,
  workspaceDiff: (input: GetWorkspaceDiffInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "diff",
      input.base,
      input.head,
    ] as const,
  workspaceVersioningPolicy: (input: GetWorkspaceVersioningPolicyInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "versioning",
    ] as const,
  fileHistory: (input: GetFileHistoryInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "file-history",
      input.path,
      input.direction ?? "desc",
      input.limit ?? 50,
      input.cursor ?? "",
    ] as const,
  fileVersionContent: (input: GetFileVersionContentInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "file-version-content",
      input.path,
      "versionId" in input ? input.versionId : `${input.fileId}@${input.ordinal}`,
    ] as const,
  mcpTokens: (databaseId: string | null, workspaceId: string) =>
    [...afsKeys.all, "databases", databaseId ?? "all", "workspaces", workspaceId, "mcp-tokens"] as const,
  allMcpTokens: () => [...afsKeys.all, "mcp-tokens", "all"] as const,
  controlPlaneTokens: () => [...afsKeys.all, "mcp-tokens", "control-plane"] as const,
};

export function databasesQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.databases(),
    queryFn: () => afsApi.listDatabases(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function accountQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.account(),
    queryFn: () => afsApi.getAccount(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function adminOverviewQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.adminOverview(),
    queryFn: () => afsApi.getAdminOverview(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function adminUsersQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.adminUsers(),
    queryFn: () => afsApi.listAdminUsers(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function adminDatabasesQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.adminDatabases(),
    queryFn: () => afsApi.listAdminDatabases(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function adminWorkspaceSummariesQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.adminWorkspaceSummaries(),
    queryFn: () => afsApi.listAdminWorkspaceSummaries(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function adminAgentsQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.adminAgents(),
    queryFn: () => afsApi.listAdminAgents(),
    staleTime: AGENT_QUERY_STALE_MS,
    gcTime: AGENT_QUERY_GC_MS,
  });
}

export function workspaceSummariesQueryOptions(databaseId: string | null) {
  return queryOptions({
    queryKey: afsKeys.workspaceSummaries(databaseId),
    queryFn: () => afsApi.listWorkspaceSummaries(databaseId ?? ""),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function workspaceQueryOptions(databaseId: string | null, workspaceId: string) {
  return queryOptions({
    queryKey: afsKeys.workspace(databaseId, workspaceId),
    queryFn: () => afsApi.getWorkspace(databaseId ?? "", workspaceId),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function agentsQueryOptions(databaseId: string | null) {
  return queryOptions({
    queryKey: afsKeys.agents(databaseId),
    queryFn: () => afsApi.listAgents(databaseId ?? ""),
    staleTime: AGENT_QUERY_STALE_MS,
    gcTime: AGENT_QUERY_GC_MS,
  });
}

export function activityQueryOptions(input: ListActivityInput) {
  return queryOptions({
    queryKey: afsKeys.activity(input),
    queryFn: () => afsApi.listActivityPage(input),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function activityItemsQueryOptions(databaseId: string | null, limit: number) {
  return queryOptions({
    queryKey: [...afsKeys.activity({ databaseId: databaseId ?? undefined, limit }), "items"] as const,
    queryFn: () => afsApi.listActivity(databaseId ?? "", limit),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function eventsQueryOptions(input: ListEventsInput) {
  return queryOptions({
    queryKey: afsKeys.events(input),
    queryFn: () => afsApi.listEvents(input),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function changelogQueryOptions(input: ListChangelogInput) {
  return queryOptions({
    queryKey: afsKeys.changelog(input),
    queryFn: () => afsApi.listChangelog(input),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function changelogInfiniteQueryOptions(input: InfiniteChangelogInput) {
  return infiniteQueryOptions({
    queryKey: afsKeys.changelogFeed(input),
    queryFn: ({ pageParam }) =>
      afsApi.listChangelog({
        ...input,
        ...((input.direction ?? "desc") === "asc"
          ? { since: typeof pageParam === "string" ? pageParam : undefined }
          : { until: typeof pageParam === "string" ? pageParam : undefined }),
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor || undefined,
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function workspaceTreeQueryOptions(input: GetWorkspaceTreeInput) {
  return queryOptions({
    queryKey: afsKeys.workspaceTree(input),
    queryFn: () => afsApi.getWorkspaceTree(input),
    staleTime: FILESYSTEM_QUERY_STALE_MS,
    gcTime: FILESYSTEM_QUERY_GC_MS,
  });
}

export function workspaceFileContentQueryOptions(input: GetWorkspaceFileContentInput) {
  return queryOptions({
    queryKey: afsKeys.workspaceFile(input),
    queryFn: () => afsApi.getWorkspaceFileContent(input),
    staleTime: FILESYSTEM_QUERY_STALE_MS,
    gcTime: FILESYSTEM_QUERY_GC_MS,
  });
}

export function workspaceDiffQueryOptions(input: GetWorkspaceDiffInput) {
  return queryOptions({
    queryKey: afsKeys.workspaceDiff(input),
    queryFn: () => afsApi.getWorkspaceDiff(input),
    staleTime: FILESYSTEM_QUERY_STALE_MS,
    gcTime: FILESYSTEM_QUERY_GC_MS,
  });
}

export function workspaceVersioningPolicyQueryOptions(input: GetWorkspaceVersioningPolicyInput) {
  return queryOptions({
    queryKey: afsKeys.workspaceVersioningPolicy(input),
    queryFn: () => afsApi.getWorkspaceVersioningPolicy(input),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function fileHistoryQueryOptions(input: GetFileHistoryInput) {
  return queryOptions({
    queryKey: afsKeys.fileHistory(input),
    queryFn: () => afsApi.getFileHistory(input),
    staleTime: FILESYSTEM_QUERY_STALE_MS,
    gcTime: FILESYSTEM_QUERY_GC_MS,
  });
}

export function fileVersionContentQueryOptions(input: GetFileVersionContentInput) {
  return queryOptions({
    queryKey: afsKeys.fileVersionContent(input),
    queryFn: () => afsApi.getFileVersionContent(input),
    staleTime: FILESYSTEM_QUERY_STALE_MS,
    gcTime: FILESYSTEM_QUERY_GC_MS,
  });
}

export function mcpTokensQueryOptions(databaseId: string | null, workspaceId: string) {
  return queryOptions({
    queryKey: afsKeys.mcpTokens(databaseId, workspaceId),
    queryFn: () => afsApi.listMCPAccessTokens(databaseId ?? undefined, workspaceId),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function allMCPAccessTokensQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.allMcpTokens(),
    queryFn: () => afsApi.listAllMCPAccessTokens(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function controlPlaneTokensQueryOptions() {
  return queryOptions({
    queryKey: afsKeys.controlPlaneTokens(),
    queryFn: () => afsApi.listControlPlaneTokens(),
    staleTime: LIVE_QUERY_STALE_MS,
    gcTime: LIVE_QUERY_GC_MS,
  });
}

export function useDatabases(enabled = true) {
  return useQuery({
    ...databasesQueryOptions(),
    enabled,
  });
}

export function useAccount(enabled = true) {
  return useQuery({
    ...accountQueryOptions(),
    enabled,
  });
}

export function useAdminOverview(enabled = true) {
  return useQuery({
    ...adminOverviewQueryOptions(),
    enabled,
  });
}

export function useAdminUsers(enabled = true) {
  return useQuery({
    ...adminUsersQueryOptions(),
    enabled,
  });
}

export function useAdminDatabases(enabled = true) {
  return useQuery({
    ...adminDatabasesQueryOptions(),
    enabled,
  });
}

export function useAdminWorkspaceSummaries(enabled = true) {
  return useQuery({
    ...adminWorkspaceSummariesQueryOptions(),
    enabled,
  });
}

export function useAdminAgents(enabled = true) {
  return useQuery({
    ...adminAgentsQueryOptions(),
    enabled,
  });
}

export function useWorkspaceSummaries(databaseId: string | null, enabled = true) {
  return useQuery(
    {
      ...workspaceSummariesQueryOptions(databaseId),
      enabled,
    },
  );
}

export function useWorkspace(databaseId: string | null, workspaceId: string, enabled = true) {
  return useQuery(
    {
      ...workspaceQueryOptions(databaseId, workspaceId),
      enabled: enabled && workspaceId !== "",
    },
  );
}

export function useMCPAccessTokens(databaseId: string | null, workspaceId: string, enabled = true) {
  return useQuery({
    ...mcpTokensQueryOptions(databaseId, workspaceId),
    enabled,
  });
}

export function useAllMCPAccessTokens(enabled = true) {
  return useQuery({
    ...allMCPAccessTokensQueryOptions(),
    enabled,
  });
}

export function useControlPlaneTokens(enabled = true) {
  return useQuery({
    ...controlPlaneTokensQueryOptions(),
    enabled,
  });
}

export function useAgents(databaseId: string | null, enabled = true) {
  return useQuery(
    {
      ...agentsQueryOptions(databaseId),
      enabled,
    },
  );
}

export function useActivity(databaseId: string | null, limit = 50, enabled = true) {
  return useQuery(
    {
      ...activityItemsQueryOptions(databaseId, limit),
      enabled,
    },
  );
}

export function useActivityPage(input: ListActivityInput, enabled = true) {
  return useQuery(
    {
      ...activityQueryOptions(input),
      enabled,
    },
  );
}

export function useEvents(input: ListEventsInput, enabled = true) {
  return useQuery(
    {
      ...eventsQueryOptions(input),
      enabled,
    },
  );
}

export function useMonitorStreamInvalidation(enabled = true) {
  const queryClient = useQueryClient();
  const pendingFamiliesRef = useRef<Set<string>>(new Set());
  const invalidateTimerRef = useRef<number | null>(null);

  useEffect(() => {
    if (!enabled || getAFSClientMode() !== "http" || typeof EventSource === "undefined") {
      return;
    }

    const familiesForMonitorEvent = (event: MessageEvent): Set<string> => {
      const families = new Set<string>();
      let payload: MonitorStreamEvent | null = null;
      try {
        payload = JSON.parse(event.data) as MonitorStreamEvent;
      } catch {
        payload = null;
      }

      const eventType = payload?.type ?? "";
      const reason = payload?.reason ?? "";
      if (eventType === "workspaces" || eventType.startsWith("workspaces/")) {
        families.add("workspaces");
        families.add("databases");
        if (reason === "deleted" || eventType === "workspaces/deleted") {
          families.add("agents");
        }
      } else if (eventType === "agents" || eventType.startsWith("agents/")) {
        families.add("agents");
        families.add("databases");
      } else if (eventType === "activity" || eventType.startsWith("activity/")) {
        families.add("activity");
        families.add("events");
      } else if (eventType === "changes" || eventType.startsWith("changes/")) {
        families.add("events");
        families.add("databases");
        families.add("workspaces");
      } else if (eventType === "mcp-tokens" || eventType.startsWith("mcp-tokens/")) {
        families.add("mcp-tokens");
      } else {
        families.add("agents");
        families.add("activity");
        families.add("events");
        families.add("workspaces");
        families.add("databases");
        families.add("mcp-tokens");
      }

      return families;
    };

    const source = new EventSource(monitorStreamURL());
    const flushInvalidation = () => {
      invalidateTimerRef.current = null;
      const pendingFamilies = pendingFamiliesRef.current;
      if (pendingFamilies.size === 0) {
        return;
      }
      pendingFamiliesRef.current = new Set();

      void queryClient.invalidateQueries({
        predicate: (query) => {
          if (!Array.isArray(query.queryKey) || query.queryKey[0] !== "afs") {
            return false;
          }
          const family = query.queryKey[1];
          return typeof family === "string" && pendingFamilies.has(family);
        },
      });
    };

    const invalidateMonitorData = (event: MessageEvent) => {
      familiesForMonitorEvent(event).forEach((family) => {
        pendingFamiliesRef.current.add(family);
      });

      if (invalidateTimerRef.current != null) {
        return;
      }
      invalidateTimerRef.current = window.setTimeout(
        flushInvalidation,
        MONITOR_INVALIDATION_DEBOUNCE_MS,
      );
    };

    source.addEventListener("monitor", invalidateMonitorData);
    return () => {
      source.removeEventListener("monitor", invalidateMonitorData);
      source.close();
      if (invalidateTimerRef.current != null) {
        window.clearTimeout(invalidateTimerRef.current);
        invalidateTimerRef.current = null;
      }
      pendingFamiliesRef.current = new Set();
    };
  }, [enabled, queryClient]);
}

export function useAgentLeaseExpiryInvalidation(agents: AFSAgentSession[], enabled = true) {
  const queryClient = useQueryClient();
  const leaseKey = agents.map((agent) => `${agent.sessionId}:${agent.leaseExpiresAt}`).join("|");

  useEffect(() => {
    if (!enabled || agents.length === 0) {
      return;
    }

    const now = Date.now();
    const nextExpiry = agents
      .filter((agent) => agent.state === "active")
      .map((agent) => Date.parse(agent.leaseExpiresAt))
      .filter((value) => Number.isFinite(value) && value > now)
      .sort((left, right) => left - right)[0];

    if (!Number.isFinite(nextExpiry)) {
      return;
    }

    const timeout = window.setTimeout(() => {
      void queryClient.invalidateQueries({
        predicate: (query) => {
          if (!Array.isArray(query.queryKey) || query.queryKey[0] !== "afs") {
            return false;
          }
          const family = query.queryKey[1];
          return family === "agents" || family === "workspaces" || family === "databases";
        },
      });
    }, Math.max(1_000, nextExpiry - now + 1_000));

    return () => window.clearTimeout(timeout);
  }, [agents, enabled, leaseKey, queryClient]);
}

export function useChangelog(input: ListChangelogInput, enabled = true) {
  return useQuery(
    {
      ...changelogQueryOptions(input),
      enabled,
    },
  );
}

export function useInfiniteChangelog(input: InfiniteChangelogInput, enabled = true) {
  return useInfiniteQuery({
    ...changelogInfiniteQueryOptions(input),
    enabled,
  });
}

export function useWorkspaceTree(input: GetWorkspaceTreeInput, enabled = true) {
  return useQuery(
    {
      ...workspaceTreeQueryOptions(input),
      enabled: enabled && input.workspaceId !== "",
    },
  );
}

export function useWorkspaceFileContent(input: GetWorkspaceFileContentInput, enabled = true) {
  return useQuery(
    {
      ...workspaceFileContentQueryOptions(input),
      enabled: enabled && input.workspaceId !== "",
    },
  );
}

export function useWorkspaceDiff(input: GetWorkspaceDiffInput, enabled = true) {
  return useQuery(
    {
      ...workspaceDiffQueryOptions(input),
      enabled: enabled && input.workspaceId !== "",
    },
  );
}

export function useWorkspaceVersioningPolicy(input: GetWorkspaceVersioningPolicyInput, enabled = true) {
  return useQuery({
    ...workspaceVersioningPolicyQueryOptions(input),
    enabled: enabled && input.workspaceId !== "",
  });
}

export function useFileHistory(input: GetFileHistoryInput, enabled = true) {
  return useQuery({
    ...fileHistoryQueryOptions(input),
    enabled: enabled && input.workspaceId !== "" && input.path.trim() !== "",
  });
}

export function useFileVersionContent(input: GetFileVersionContentInput, enabled = true) {
  return useQuery({
    ...fileVersionContentQueryOptions(input),
    enabled: enabled && input.workspaceId !== "" && input.path.trim() !== "",
  });
}

function useWorkspaceInvalidation() {
  const queryClient = useQueryClient();

  return async () => {
    await queryClient.invalidateQueries({
      predicate: (query) => Array.isArray(query.queryKey) && query.queryKey[0] === "afs",
    });
  };
}

export function useSaveDatabaseMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: SaveDatabaseInput) => afsApi.saveDatabase(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useCreateMCPAccessTokenMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateMCPTokenInput) => afsApi.createMCPAccessToken(input),
    onSuccess: (token, variables) => {
      void queryClient.invalidateQueries({
        queryKey: afsKeys.mcpTokens(variables.databaseId ?? null, variables.workspaceId),
      });
      void queryClient.invalidateQueries({ queryKey: afsKeys.allMcpTokens() });
      void queryClient.invalidateQueries({ queryKey: afsKeys.agents(variables.databaseId ?? null) });
    },
  });
}

export function useRevokeMCPAccessTokenMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { databaseId?: string; workspaceId: string; tokenId: string }) =>
      afsApi.revokeMCPAccessToken(input.databaseId, input.workspaceId, input.tokenId),
    onSuccess: (_, variables) => {
      void queryClient.invalidateQueries({
        queryKey: afsKeys.mcpTokens(variables.databaseId ?? null, variables.workspaceId),
      });
      void queryClient.invalidateQueries({ queryKey: afsKeys.allMcpTokens() });
      void queryClient.invalidateQueries({ queryKey: afsKeys.agents(variables.databaseId ?? null) });
    },
  });
}

export function useCreateControlPlaneTokenMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateControlPlaneTokenInput) => afsApi.createControlPlaneToken(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: afsKeys.controlPlaneTokens() });
    },
  });
}

export function useRevokeControlPlaneTokenMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (tokenId: string) => afsApi.revokeControlPlaneToken(tokenId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: afsKeys.controlPlaneTokens() });
    },
  });
}

export function useDeleteDatabaseMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (databaseId: string) => afsApi.deleteDatabase(databaseId),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useSetDefaultDatabaseMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (databaseId: string) => afsApi.setDefaultDatabase(databaseId),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useResetAccountDataMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: () => afsApi.resetAccountData(),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useDeleteAccountMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: () => afsApi.deleteAccount(),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useCreateWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateWorkspaceInput) => afsApi.createWorkspace(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useDeleteWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: { databaseId?: string; workspaceId: string }) =>
      afsApi.deleteWorkspace(input.databaseId ?? "", input.workspaceId),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useUpdateWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateWorkspaceInput) => afsApi.updateWorkspace(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useUpdateWorkspaceFileMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateWorkspaceFileInput) => afsApi.updateWorkspaceFile(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useUpdateWorkspaceVersioningPolicyMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateWorkspaceVersioningPolicyInput) => afsApi.updateWorkspaceVersioningPolicy(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useDiffFileVersionsMutation() {
  return useMutation({
    mutationFn: (input: DiffFileVersionsInput) => afsApi.diffFileVersions(input),
  });
}

export function useRestoreFileVersionMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: RestoreFileVersionInput) => afsApi.restoreFileVersion(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useUndeleteFileVersionMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UndeleteFileVersionInput) => afsApi.undeleteFileVersion(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useCreateSavepointMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateSavepointInput) => afsApi.createSavepoint(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useRestoreSavepointMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: RestoreSavepointInput) => afsApi.restoreSavepoint(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useQuickstartMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: QuickstartInput) => afsApi.quickstart(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useImportLocalMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: ImportLocalInput) => afsApi.importLocal(input),
    onSuccess: async () => {
      await invalidate();
    },
  });
}
