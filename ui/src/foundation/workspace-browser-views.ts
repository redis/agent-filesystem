import type { AFSWorkspaceDetail, AFSWorkspaceView } from "./types/afs";

export type WorkspaceBrowserViewOption = {
  value: AFSWorkspaceView;
  label: string;
};

export function hasDistinctWorkingCopy(workspace: AFSWorkspaceDetail) {
  return workspace.capabilities.browseWorkingCopy && workspace.draftState === "dirty";
}

export function getWorkspaceBrowserViewOptions(workspace: AFSWorkspaceDetail): WorkspaceBrowserViewOption[] {
  const options: WorkspaceBrowserViewOption[] = [];

  if (hasDistinctWorkingCopy(workspace)) {
    options.push({ value: "working-copy", label: "working-copy" });
  }

  if (workspace.capabilities.browseHead || options.length === 0) {
    options.push({ value: "head", label: "head" });
  }

  if (workspace.capabilities.browseCheckpoints) {
    for (const savepoint of workspace.savepoints) {
      if (savepoint.id === workspace.headSavepointId) {
        continue;
      }
      options.push({
        value: `checkpoint:${savepoint.id}`,
        label: savepoint.name,
      });
    }
  }

  return options;
}

export function getDefaultWorkspaceBrowserView(workspace: AFSWorkspaceDetail): AFSWorkspaceView {
  return getWorkspaceBrowserViewOptions(workspace)[0]?.value ?? "head";
}
