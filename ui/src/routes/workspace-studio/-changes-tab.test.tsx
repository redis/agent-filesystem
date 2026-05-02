import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import { ChangesTab } from "./-changes-tab";

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

const rows = [
  {
    id: "chg-1",
    op: "put",
    path: "/src/app.ts",
    versionId: "ver-1234567890",
    fileId: "file-1234567890",
  },
];

vi.mock("../../foundation/hooks/use-afs", () => ({
  useInfiniteChangelog: () => ({
    isLoading: false,
    isError: false,
    hasNextPage: false,
    isFetchingNextPage: false,
    data: {
      pages: [{ entries: rows }],
    },
  }),
}));

vi.mock("../../foundation/tables/changes-table", () => ({
  ChangesTable: ({
    rows: tableRows,
    onOpenChange,
  }: {
    rows: Array<{ path: string; versionId?: string }>;
    onOpenChange?: (entry: { path: string; versionId?: string }) => void;
  }) => (
    <button type="button" onClick={() => onOpenChange?.(tableRows[0])}>
      open change
    </button>
  ),
}));

vi.mock("./-file-history-drawer", () => ({
  FileHistoryDrawer: ({
    path,
    initialVersionId,
  }: {
    path: string;
    initialVersionId?: string;
  }) => <div data-testid="history-drawer">{`${path}::${initialVersionId ?? ""}`}</div>,
}));

describe("ChangesTab version deep links", () => {
  test("opens the file history drawer anchored to the changelog version", () => {
    render(<ChangesTab databaseId="db-1" workspaceId="workspace-1" editable />);

    fireEvent.click(screen.getByRole("button", { name: /open change/i }));

    expect(screen.getByTestId("history-drawer")).toHaveTextContent("/src/app.ts::ver-1234567890");
  });
});
