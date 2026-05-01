import type { AFSWorkspaceDetail, AFSWorkspaceView } from "./types/afs";

export type WorkspaceBrowserViewOption = {
  value: AFSWorkspaceView;
  label: string;
};

export function hasDistinctWorkingCopy(workspace: AFSWorkspaceDetail) {
  return workspace.capabilities.browseWorkingCopy && workspace.draftState === "dirty";
}

export function getActiveWorkspaceView(workspace: AFSWorkspaceDetail): AFSWorkspaceView {
  return hasDistinctWorkingCopy(workspace) ? "working-copy" : "head";
}

export function getWorkspaceBrowserViewOptions(workspace: AFSWorkspaceDetail): WorkspaceBrowserViewOption[] {
  const options: WorkspaceBrowserViewOption[] = [];
  const hasDirtyActiveState = hasDistinctWorkingCopy(workspace);
  const activeView = getActiveWorkspaceView(workspace);

  if (activeView === "working-copy" || workspace.capabilities.browseHead || options.length === 0) {
    options.push({ value: activeView, label: "Active workspace" });
  }

  if (workspace.capabilities.browseCheckpoints) {
    for (const savepoint of workspace.savepoints) {
      if (!hasDirtyActiveState && savepoint.id === workspace.headSavepointId) {
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
  return getWorkspaceBrowserViewOptions(workspace)[0]?.value ?? getActiveWorkspaceView(workspace);
}

export function resolveWorkspaceBrowserView(
  workspace: AFSWorkspaceDetail,
  requestedView: AFSWorkspaceView,
): AFSWorkspaceView {
  return getWorkspaceBrowserViewOptions(workspace).some((option) => option.value === requestedView)
    ? requestedView
    : getDefaultWorkspaceBrowserView(workspace);
}
