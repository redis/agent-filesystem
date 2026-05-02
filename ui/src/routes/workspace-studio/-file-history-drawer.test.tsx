import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import { FileHistoryDrawer } from "./-file-history-drawer";

vi.mock("@redis-ui/components", () => ({
  Button: Object.assign((props: any) => <button {...props} />, {
    defaultProps: {
      theme: {
        semantic: {
          color: {
            background: {
              danger500: "#dc2626",
              danger600: "#b91c1c",
            },
            text: {
              inverse: "#ffffff",
            },
          },
        },
      },
    },
  }),
  Card: (props: any) => <div {...props} />,
  Typography: {
    Body: (props: any) => <span {...props} />,
    Heading: (props: any) => <h2 {...props} />,
  },
}));

const changelogEntries = [
  {
    id: "chg-2",
    occurredAt: "2026-04-30T12:00:00Z",
    op: "rename",
    path: "/README.md",
    prevPath: "/README.txt",
    versionId: "ver-2",
    fileId: "file-1",
    label: "agent-one",
  },
  {
    id: "chg-1",
    occurredAt: "2026-04-30T11:00:00Z",
    op: "put",
    path: "/README.md",
    versionId: "ver-1",
    fileId: "file-1",
    label: "agent-one",
  },
];

const historyData = {
  workspaceId: "workspace-1",
  path: "/README.md",
  order: "desc" as const,
  lineages: [
    {
      fileId: "file-1",
      currentPath: "/README.md",
      state: "active",
      versions: [
        {
          versionId: "ver-2",
          fileId: "file-1",
          ordinal: 2,
          op: "rename",
          kind: "text",
          path: "/README.md",
          prevPath: "/README.txt",
          createdAt: "2026-04-30T12:00:00Z",
          source: "sync_upload",
          sizeBytes: 14,
        },
        {
          versionId: "ver-1",
          fileId: "file-1",
          ordinal: 1,
          op: "put",
          kind: "text",
          path: "/README.md",
          createdAt: "2026-04-30T11:00:00Z",
          source: "sync_upload",
          sizeBytes: 12,
        },
      ],
    },
  ],
  nextCursor: "",
};

const useChangelogMock = vi.fn(() => ({
  isLoading: false,
  isError: false,
  data: {
    entries: changelogEntries,
  },
}));

const useFileHistoryMock = vi.fn(() => ({
  isLoading: false,
  isError: false,
  data: historyData,
}));

const useFileVersionContentMock = vi.fn((input: any) => {
  const versionId = "versionId" in input && input.versionId
    ? input.versionId
    : input.ordinal === 1
      ? "ver-1"
      : "ver-2";
  return {
    isLoading: false,
    isError: false,
    data: {
      kind: "text",
      binary: false,
      content: versionId === "ver-1" ? "first revision" : "second revision",
    },
  };
});

vi.mock("../../foundation/hooks/use-afs", () => ({
  useChangelog: (...args: any[]) => useChangelogMock(...args),
  useFileHistory: (...args: any[]) => useFileHistoryMock(...args),
  useFileVersionContent: (...args: any[]) => useFileVersionContentMock(...args),
  useDiffFileVersionsMutation: () => ({
    isPending: false,
    isError: false,
    data: null,
    mutateAsync: vi.fn(),
  }),
  useRestoreFileVersionMutation: () => ({
    isPending: false,
    mutateAsync: vi.fn(),
  }),
  useUndeleteFileVersionMutation: () => ({
    isPending: false,
    mutateAsync: vi.fn(),
  }),
}));

describe("FileHistoryDrawer merged history surfaces", () => {
  test("shows path activity next to file history and lets activity rows select linked versions", () => {
    render(
      <FileHistoryDrawer
        databaseId="db-1"
        workspaceId="workspace-1"
        path="/README.md"
        editable
        initialVersionId="ver-1"
        onClose={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Path activity", level: 3 })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "File history", level: 3 })).toBeInTheDocument();
    expect(screen.getByText("first revision")).toBeInTheDocument();

    const activityButtons = screen.getAllByText("agent-one");
    fireEvent.click(activityButtons[0].closest("button") as HTMLButtonElement);

    expect(screen.getByRole("heading", { name: "ver-2", level: 3 })).toBeInTheDocument();
    expect(screen.getByText("second revision")).toBeInTheDocument();
  });
});
