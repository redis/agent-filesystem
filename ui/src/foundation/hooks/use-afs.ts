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
  QuickstartInput,
  ImportLocalInput,
} from "../types/afs";

const afsKeys = {
  all: ["afs"] as const,
  databases: () => [...afsKeys.all, "databases"] as const,
  workspaceSummaries: (databaseId: string | null) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", "summaries"] as const,
  workspace: (databaseId: string | null, workspaceId: string) =>
    [...afsKeys.all, "workspaces", databaseId ?? "all", workspaceId] as const,
  agents: (databaseId: string | null) =>
    [...afsKeys.all, "agents", databaseId ?? "all"] as const,
  activity: (databaseId: string | null, limit: number) =>
    [...afsKeys.all, "activity", databaseId ?? "all", limit] as const,
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
      queryKey: afsKeys.workspaceSummaries(databaseId),
      queryFn: () => afsApi.listWorkspaceSummaries(databaseId ?? ""),
      enabled,
    }),
  );
}

export function useWorkspace(databaseId: string | null, workspaceId: string, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspace(databaseId, workspaceId),
      queryFn: () => afsApi.getWorkspace(databaseId ?? "", workspaceId),
      enabled: enabled && workspaceId !== "",
    }),
  );
}

export function useAgents(databaseId: string | null, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.agents(databaseId),
      queryFn: () => afsApi.listAgents(databaseId ?? ""),
      enabled,
    }),
  );
}

export function useActivity(databaseId: string | null, limit = 50, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.activity(databaseId, limit),
      queryFn: () => afsApi.listActivity(databaseId ?? "", limit),
      enabled,
    }),
  );
}

export function useWorkspaceTree(input: GetWorkspaceTreeInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceTree(input),
      queryFn: () => afsApi.getWorkspaceTree(input),
      enabled: enabled && input.workspaceId !== "",
    }),
  );
}

export function useWorkspaceFileContent(input: GetWorkspaceFileContentInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceFile(input),
      queryFn: () => afsApi.getWorkspaceFileContent(input),
      enabled: enabled && input.workspaceId !== "",
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

export function useSetDefaultDatabaseMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (databaseId: string) => afsApi.setDefaultDatabase(databaseId),
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
