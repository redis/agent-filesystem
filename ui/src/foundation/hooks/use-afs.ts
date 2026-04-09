import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { afsApi } from "../api/afs";
import type {
  CreateSavepointInput,
  CreateWorkspaceInput,
  GetWorkspaceFileContentInput,
  GetWorkspaceTreeInput,
  RestoreSavepointInput,
  SaveDatabaseInput,
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
} from "../types/afs";

const afsKeys = {
  all: ["afs"] as const,
  databases: () => [...afsKeys.all, "databases"] as const,
  workspaceSummaries: (databaseId: string) =>
    [...afsKeys.all, "databases", databaseId, "workspace-summaries"] as const,
  workspace: (databaseId: string, workspaceId: string) =>
    [...afsKeys.all, "databases", databaseId, "workspaces", workspaceId] as const,
  activity: (databaseId: string, limit: number) =>
    [...afsKeys.all, "databases", databaseId, "activity", limit] as const,
  workspaceTree: (input: GetWorkspaceTreeInput) =>
    [
      ...afsKeys.all,
      "databases",
      input.databaseId,
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
      input.databaseId,
      "workspaces",
      input.workspaceId,
      "files",
      input.view,
      input.path,
    ] as const,
};

export function useDatabases() {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.databases(),
      queryFn: () => afsApi.listDatabases(),
    }),
  );
}

export function useWorkspaceSummaries(databaseId: string | null, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceSummaries(databaseId ?? "none"),
      queryFn: () => afsApi.listWorkspaceSummaries(databaseId ?? ""),
      enabled: enabled && databaseId != null && databaseId !== "",
    }),
  );
}

export function useWorkspace(databaseId: string | null, workspaceId: string, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspace(databaseId ?? "none", workspaceId),
      queryFn: () => afsApi.getWorkspace(databaseId ?? "", workspaceId),
      enabled: enabled && databaseId != null && databaseId !== "" && workspaceId !== "",
    }),
  );
}

export function useActivity(databaseId: string | null, limit = 50, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.activity(databaseId ?? "none", limit),
      queryFn: () => afsApi.listActivity(databaseId ?? "", limit),
      enabled: enabled && databaseId != null && databaseId !== "",
    }),
  );
}

export function useWorkspaceTree(input: GetWorkspaceTreeInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceTree(input),
      queryFn: () => afsApi.getWorkspaceTree(input),
      enabled: enabled && input.databaseId !== "" && input.workspaceId !== "",
    }),
  );
}

export function useWorkspaceFileContent(input: GetWorkspaceFileContentInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceFile(input),
      queryFn: () => afsApi.getWorkspaceFileContent(input),
      enabled: enabled && input.databaseId !== "" && input.workspaceId !== "",
    }),
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
    mutationFn: (input: { databaseId: string; workspaceId: string }) =>
      afsApi.deleteWorkspace(input.databaseId, input.workspaceId),
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
