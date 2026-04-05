import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { rafApi } from "../api/raf";
import type {
  CreateSavepointInput,
  CreateSessionInput,
  CreateWorkspaceInput,
  RollbackSessionInput,
  UpdateSessionFileInput,
} from "../types/raf";

const rafKeys = {
  all: ["raf"] as const,
  workspaceSummaries: () => [...rafKeys.all, "workspace-summaries"] as const,
  workspaces: () => [...rafKeys.all, "workspaces"] as const,
  workspace: (workspaceId: string) => [...rafKeys.all, "workspaces", workspaceId] as const,
};

export function useWorkspaceSummaries() {
  return useQuery(
    queryOptions({
      queryKey: rafKeys.workspaceSummaries(),
      queryFn: () => rafApi.listWorkspaceSummaries(),
    }),
  );
}

export function useWorkspaces() {
  return useQuery(
    queryOptions({
      queryKey: rafKeys.workspaces(),
      queryFn: () => rafApi.listWorkspaces(),
    }),
  );
}

export function useWorkspace(workspaceId: string) {
  return useQuery(
    queryOptions({
      queryKey: rafKeys.workspace(workspaceId),
      queryFn: () => rafApi.getWorkspace(workspaceId),
    }),
  );
}

function useWorkspaceInvalidation() {
  const queryClient = useQueryClient();

  return (workspaceId?: string) =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: rafKeys.workspaceSummaries() }),
      queryClient.invalidateQueries({ queryKey: rafKeys.workspaces() }),
      workspaceId == null
        ? Promise.resolve()
        : queryClient.invalidateQueries({ queryKey: rafKeys.workspace(workspaceId) }),
    ]);
}

export function useCreateWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateWorkspaceInput) => rafApi.createWorkspace(input),
    onSuccess: async (workspace) => {
      await invalidate(workspace.id);
    },
  });
}

export function useDeleteWorkspaceMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (workspaceId: string) => rafApi.deleteWorkspace(workspaceId),
    onSuccess: async () => {
      await invalidate();
    },
  });
}

export function useCreateSessionMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateSessionInput) => rafApi.createSession(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useDeleteSessionMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: { workspaceId: string; sessionId: string }) =>
      rafApi.deleteSession(input.workspaceId, input.sessionId),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useUpdateSessionFileMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: UpdateSessionFileInput) => rafApi.updateSessionFile(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useCreateSavepointMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: CreateSavepointInput) => rafApi.createSavepoint(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}

export function useRollbackSessionMutation() {
  const invalidate = useWorkspaceInvalidation();

  return useMutation({
    mutationFn: (input: RollbackSessionInput) => rafApi.rollbackSession(input),
    onSuccess: async (_, variables) => {
      await invalidate(variables.workspaceId);
    },
  });
}
