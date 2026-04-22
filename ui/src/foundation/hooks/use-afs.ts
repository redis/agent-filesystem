import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { afsApi } from "../api/afs";
import type { ListChangelogInput } from "../api/afs";
import type {
  CreateSavepointInput,
  CreateWorkspaceInput,
  GetWorkspaceFileContentInput,
  GetWorkspaceTreeInput,
  RestoreSavepointInput,
  SaveDatabaseInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
  QuickstartInput,
  ImportLocalInput,
} from "../types/afs";

const LIVE_QUERY_STALE_MS = 10_000;
const LIVE_QUERY_GC_MS = 10 * 60 * 1000;
const AGENT_QUERY_STALE_MS = 5_000;
const AGENT_QUERY_GC_MS = 5 * 60 * 1000;
const FILESYSTEM_QUERY_STALE_MS = 30_000;
const FILESYSTEM_QUERY_GC_MS = 5 * 60 * 1000;

export const afsKeys = {
  all: ["afs"] as const,
  account: () => [...afsKeys.all, "account"] as const,
  databases: () => [...afsKeys.all, "databases"] as const,
  workspaceSummaries: (databaseId: string | null) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", "summaries"] as const,
  workspace: (databaseId: string | null, workspaceId: string) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", workspaceId] as const,
  agents: (databaseId: string | null) =>
    [...afsKeys.all, "agents", databaseId ?? "all"] as const,
  activity: (databaseId: string | null, limit: number) =>
    [...afsKeys.all, "activity", databaseId ?? "all", limit] as const,
  changelog: (input: ListChangelogInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId ?? "all",
      "workspaces",
      input.workspaceId,
      "changes",
      input.sessionId ?? "all",
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

export function activityQueryOptions(databaseId: string | null, limit: number) {
  return queryOptions({
    queryKey: afsKeys.activity(databaseId, limit),
    queryFn: () => afsApi.listActivity(databaseId ?? "", limit),
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
      ...activityQueryOptions(databaseId, limit),
      enabled,
    },
  );
}

export function useChangelog(input: ListChangelogInput, enabled = true) {
  return useQuery(
    {
      ...changelogQueryOptions(input),
      enabled: enabled && input.workspaceId !== "",
    },
  );
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
