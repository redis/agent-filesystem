import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, test, vi } from "vitest";
import { ActivityTab } from "./-activity-tab";

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

const useActivityPageMock = vi.fn(() => ({
  isLoading: false,
  isError: false,
  data: {
    items: [
      {
        id: "evt-1",
        createdAt: "2026-04-30T12:00:00Z",
        actor: "agent",
        detail: "Updated /README.md",
        kind: "put",
        path: "/README.md",
        scope: "file",
        title: "Updated README",
      },
    ],
  },
}));

vi.mock("../../foundation/hooks/use-afs", () => ({
  useActivityPage: (...args: any[]) => useActivityPageMock(...args),
}));

vi.mock("../../foundation/tables/activity-table", () => ({
  ActivityTable: ({
    rows,
    onOpenActivity,
  }: {
    rows: Array<Record<string, unknown>>;
    onOpenActivity: (row: Record<string, unknown>) => void;
  }) => (
    <button type="button" onClick={() => onOpenActivity(rows[0])}>
      open activity
    </button>
  ),
}));

describe("ActivityTab workspace history", () => {
  test("loads workspace-scoped history and routes file activity back to browse", () => {
    const onTabChange = vi.fn();

    render(
      <ActivityTab
        databaseId="db-1"
        workspaceId="workspace-1"
        updatedAt="2026-04-30T12:30:00Z"
        onTabChange={onTabChange}
      />,
    );

    expect(screen.getByText("Workspace history")).toBeInTheDocument();
    expect(useActivityPageMock).toHaveBeenCalledWith(
      {
        databaseId: "db-1",
        workspaceId: "workspace-1",
        limit: 50,
      },
    );

    fireEvent.click(screen.getByRole("button", { name: /open activity/i }));

    expect(onTabChange).toHaveBeenCalledWith("browse");
  });
});
