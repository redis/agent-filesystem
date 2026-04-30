import type { AFSWorkspaceDetail, AFSWorkspaceView } from "../../foundation/types/afs";
import { FilesTab } from "./-files-tab";

type Props = {
  workspace: AFSWorkspaceDetail;
  browserView: AFSWorkspaceView;
  onBrowserViewChange: (view: AFSWorkspaceView) => void;
  onViewAllCheckpoints: () => void;
};

export function BrowseTab({
  workspace,
  browserView,
  onBrowserViewChange,
  onViewAllCheckpoints,
}: Props) {
  return (
    <FilesTab
      workspace={workspace}
      browserView={browserView}
      onBrowserViewChange={onBrowserViewChange}
      onViewAllCheckpoints={onViewAllCheckpoints}
    />
  );
}
