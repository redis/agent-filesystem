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
  UpdateWorkspaceInput,
  UpdateWorkspaceFileInput,
} from "../types/afs";

const afsKeys = {
  all: ["afs"] as const,
  workspaceSummaries: () => [...afsKeys.all, "workspace-summaries"] as const,
  workspace: (workspaceId: string) => [...afsKeys.all, "workspaces", workspaceId] as const,
  activity: (limit: number) => [...afsKeys.all, "activity", limit] as const,
  workspaceTree: (input: GetWorkspaceTreeInput) =>
    [...afsKeys.all, "workspaces", input.workspaceId, "tree", input.view, input.path, input.depth ?? 1] as const,
  workspaceFile: (input: GetWorkspaceFileContentInput) =>
    [...afsKeys.all, "workspaces", input.workspaceId, "files", input.view, input.path] as const,
};

export function useWorkspaceSummaries() {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceSummaries(),
      queryFn: () => afsApi.listWorkspaceSummaries(),
    }),
  );
}

export function useWorkspace(workspaceId: string, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspace(workspaceId),
      queryFn: () => afsApi.getWorkspace(workspaceId),
      enabled,
    }),
  );
}

export function useActivity(limit = 50) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.activity(limit),
      queryFn: () => afsApi.listActivity(limit),
    }),
  );
}

export function useWorkspaceTree(input: GetWorkspaceTreeInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceTree(input),
      queryFn: () => afsApi.getWorkspaceTree(input),
      enabled,
    }),
  );
}

export function useWorkspaceFileContent(input: GetWorkspaceFileContentInput, enabled = true) {
  return useQuery(
    queryOptions({
      queryKey: afsKeys.workspaceFile(input),
      queryFn: () => afsApi.getWorkspaceFileContent(input),
      enabled,
    }),
  );
}

function useWorkspaceInvalidation() {
  const queryClient = useQueryClient();

  return async (workspaceId?: string) => {
    const invalidations: Array<Promise<unknown>> = [
      queryClient.invalidateQueries({ queryKey: afsKeys.workspaceSummaries() }),
      queryClient.invalidateQueries({
        predicate: (query) =>
          Array.isArray(query.queryKey) &&
          query.queryKey[0] === "afs" &&
          query.queryKey[1] === "activity",
      }),
    ];

    if (workspaceId != null) {
      invalidations.push(queryClient.invalidateQueries({ queryKey: afsKeys.workspace(workspaceId) }));
      invalidations.push(
        queryClient.invalidateQueries({
          predicate: (query) =>
            Array.isArray(query.queryKey) &&
            query.queryKey[0] === "afs" &&
            query.queryKey[1] === "workspaces" &&
            query.queryKey[2] === workspaceId,
        }),
      );
    }

    await Promise.all(invalidations);
  };
}

export function useCreateWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateWorkspaceInput) => afsApi.createWorkspace(input),
    onSuccess: async (workspace) => {
      await invalidate(workspace.id);
    },
  });
}

export function useDeleteWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (workspaceId: string) => afsApi.deleteWorkspace(workspaceId),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useUpdateWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateWorkspaceInput) => afsApi.updateWorkspace(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useUpdateWorkspaceFileMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateWorkspaceFileInput) => afsApi.updateWorkspaceFile(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useCreateSavepointMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateSavepointInput) => afsApi.createSavepoint(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useRestoreSavepointMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: RestoreSavepointInput) => afsApi.restoreSavepoint(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}
