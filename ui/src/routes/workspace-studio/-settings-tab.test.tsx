import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { ThemeProvider } from "styled-components";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { SettingsTab } from "./-settings-tab";

const mutateAsync = vi.fn();
const versioningData = {
  mode: "paths" as const,
  includeGlobs: ["src/**"],
  excludeGlobs: ["**/*.log"],
  maxVersionsPerFile: 5,
  maxAgeDays: 30,
  maxTotalBytes: 4096,
  largeFileCutoffBytes: 1024,
};

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
  Select: ({ options, value, onChange, ...props }: any) => (
    <select
      {...props}
      value={value}
      onChange={(event) => onChange(event.currentTarget.value)}
    >
      {options.map((option: any) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
  Card: (props: any) => <div {...props} />,
  Typography: {
    Body: (props: any) => <span {...props} />,
    Heading: (props: any) => <h2 {...props} />,
  },
}));

vi.mock("../../foundation/hooks/use-afs", () => ({
  useWorkspaceVersioningPolicy: () => ({
    data: versioningData,
    isLoading: false,
    isError: false,
  }),
  useUpdateWorkspaceVersioningPolicyMutation: () => ({
    mutateAsync,
    isPending: false,
  }),
}));

afterEach(() => {
  cleanup();
});

describe("SettingsTab versioning controls", () => {
  beforeEach(() => {
    mutateAsync.mockReset();
    mutateAsync.mockResolvedValue(undefined);
  });

  test("shows Redis Array, content storage, and search index metadata", () => {
    render(
      <ThemeProvider theme={testTheme}>
        <SettingsTab
          workspace={buildWorkspace()}
          onSave={vi.fn()}
          isSaving={false}
          onDelete={vi.fn()}
          isDeleting={false}
          mcpTokens={[]}
          onOpenMCPConsole={vi.fn()}
        />
      </ThemeProvider>,
    );

    expect(screen.queryByText("Redis Array support")).not.toBeInTheDocument();
    expect(screen.queryByText("Redis keyspace")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /launch redis insight/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("db")).toBeInTheDocument();
    expect(screen.getByText("File storage")).toBeInTheDocument();
    expect(screen.getByText("Redis Array")).toBeInTheDocument();
    expect(screen.getByText("Search index")).toBeInTheDocument();
    expect(
      screen.getByText("Search index is ready with 2 indexed documents."),
    ).toBeInTheDocument();
    expect(
      screen.getByText("afs:{workspace-1}:search_index_v1"),
    ).toBeInTheDocument();
  });

  test("submits the parsed versioning policy", async () => {
    render(
      <ThemeProvider theme={testTheme}>
        <SettingsTab
          workspace={buildWorkspace()}
          onSave={vi.fn()}
          isSaving={false}
          onDelete={vi.fn()}
          isDeleting={false}
          mcpTokens={[]}
          onOpenMCPConsole={vi.fn()}
        />
      </ThemeProvider>,
    );

    await waitFor(() => {
      expect(screen.getByLabelText(/tracking mode/i)).toHaveValue("paths");
      expect(screen.getByLabelText(/include globs/i)).toHaveValue("src/**");
    });

    fireEvent.change(screen.getByLabelText(/tracking mode/i), {
      target: { value: "all" },
    });
    fireEvent.change(screen.getByLabelText(/include globs/i), {
      target: { value: "src/**\nweb/**" },
    });
    fireEvent.change(screen.getByLabelText(/max versions per file/i), {
      target: { value: "12" },
    });

    fireEvent.click(
      screen.getByRole("button", { name: /save versioning policy/i }),
    );

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        databaseId: "db-1",
        workspaceId: "workspace-1",
        policy: {
          mode: "all",
          includeGlobs: ["src/**", "web/**"],
          excludeGlobs: ["**/*.log"],
          maxVersionsPerFile: 12,
          maxAgeDays: 30,
          maxTotalBytes: 4096,
          largeFileCutoffBytes: 1024,
        },
      });
    });
  });
});

function buildWorkspace() {
  return {
    id: "workspace-1",
    name: "workspace",
    description: "",
    cloudAccount: "Direct Redis",
    databaseId: "db-1",
    databaseName: "db",
    databaseSupportsArrays: true,
    redisKey: "afs:workspace-1",
    region: "us-east-1",
    source: "blank" as const,
    createdAt: "2026-04-29T00:00:00Z",
    updatedAt: "2026-04-29T00:00:00Z",
    draftState: "clean",
    headSavepointId: "cp-1",
    tags: [],
    fileCount: 1,
    folderCount: 0,
    totalBytes: 128,
    contentStorage: {
      profile: "array" as const,
      fileCount: 2,
      arrayFileCount: 2,
      legacyFileCount: 0,
    },
    searchIndex: {
      name: "afs:{workspace-1}:search_index_v1",
      present: true,
      ready: true,
      status: "ready" as const,
      documentCount: 2,
      percentIndexed: 1,
    },
    checkpointCount: 0,
    files: [],
    savepoints: [],
    activity: [],
    agents: [],
    capabilities: {
      browseHead: true,
      browseCheckpoints: true,
      browseWorkingCopy: true,
      editWorkingCopy: true,
      createCheckpoint: true,
      restoreCheckpoint: true,
    },
  };
}

const testTheme = {
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
};
