import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { PropsWithChildren } from "react";
import { getAFSClientMode } from "./api/afs";
import { useActivity, useWorkspaceSummaries } from "./hooks/use-afs";
import type { AFSClientMode, AFSWorkspaceSummary } from "./types/afs";

const SAVED_DATABASES_STORAGE_KEY = "afs_saved_databases_v1";
const SELECTED_DATABASE_STORAGE_KEY = "afs_selected_database_v1";

export type AFSDatabaseScopeRecord = {
  id: string;
  displayName: string;
  databaseName: string;
  endpointLabel: string;
  dbIndex: string;
  workspaceCount: number;
  source: "derived" | "saved";
  updatedAt?: string;
};

export type OpenDatabaseInput = {
  displayName: string;
  databaseName: string;
  endpointLabel: string;
  dbIndex: string;
};

type SavedDatabaseRecord = {
  id: string;
  displayName: string;
  databaseName: string;
  endpointLabel: string;
  dbIndex: string;
  lastOpenedAt: string;
};

type DatabaseScopeContextValue = {
  clientMode: AFSClientMode;
  databases: AFSDatabaseScopeRecord[];
  selectedDatabase: AFSDatabaseScopeRecord | null;
  selectedDatabaseId: string | null;
  isLoading: boolean;
  selectDatabase: (databaseId: string) => void;
  openDatabase: (input: OpenDatabaseInput) => void;
  isOpenDatabaseDialogOpen: boolean;
  setOpenDatabaseDialogOpen: (value: boolean) => void;
};

const DatabaseScopeContext = createContext<DatabaseScopeContextValue | null>(null);

function slugify(value: string) {
  return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-");
}

function readSavedDatabases(): SavedDatabaseRecord[] {
  const raw = localStorage.getItem(SAVED_DATABASES_STORAGE_KEY);
  if (raw == null) {
    return [];
  }

  try {
    return JSON.parse(raw) as SavedDatabaseRecord[];
  } catch {
    localStorage.removeItem(SAVED_DATABASES_STORAGE_KEY);
    return [];
  }
}

function readSelectedDatabaseId() {
  return localStorage.getItem(SELECTED_DATABASE_STORAGE_KEY);
}

function deriveDatabases(workspaces: AFSWorkspaceSummary[]): AFSDatabaseScopeRecord[] {
  const grouped = new Map<
    string,
    {
      workspaceCount: number;
      databaseName: string;
      endpointLabel: string;
      updatedAt: string;
    }
  >();

  for (const workspace of workspaces) {
    const existing = grouped.get(workspace.databaseId);
    const nextEndpoint = `${workspace.cloudAccount} · ${workspace.region}`;

    if (existing == null) {
      grouped.set(workspace.databaseId, {
        workspaceCount: 1,
        databaseName: workspace.databaseName,
        endpointLabel: nextEndpoint,
        updatedAt: workspace.updatedAt,
      });
      continue;
    }

    grouped.set(workspace.databaseId, {
      workspaceCount: existing.workspaceCount + 1,
      databaseName: existing.databaseName,
      endpointLabel:
        workspace.updatedAt.localeCompare(existing.updatedAt) > 0
          ? nextEndpoint
          : existing.endpointLabel,
      updatedAt:
        workspace.updatedAt.localeCompare(existing.updatedAt) > 0
          ? workspace.updatedAt
          : existing.updatedAt,
    });
  }

  return [...grouped.entries()]
    .map(([id, record]) => ({
      id,
      displayName: record.databaseName,
      databaseName: record.databaseName,
      endpointLabel: record.endpointLabel,
      dbIndex: "",
      workspaceCount: record.workspaceCount,
      source: "derived" as const,
      updatedAt: record.updatedAt,
    }))
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
}

function mergeDatabases(
  derived: AFSDatabaseScopeRecord[],
  saved: SavedDatabaseRecord[],
): AFSDatabaseScopeRecord[] {
  const merged = new Map<string, AFSDatabaseScopeRecord>();

  for (const record of derived) {
    merged.set(record.id, record);
  }

  for (const record of saved) {
    const existing = merged.get(record.id);
    merged.set(record.id, {
      id: record.id,
      displayName: record.displayName.trim() || existing?.displayName || record.databaseName,
      databaseName: existing?.databaseName ?? record.databaseName,
      endpointLabel: record.endpointLabel.trim() || existing?.endpointLabel || "Opened manually",
      dbIndex: record.dbIndex,
      workspaceCount: existing?.workspaceCount ?? 0,
      source: existing?.source ?? "saved",
      updatedAt: existing?.updatedAt ?? record.lastOpenedAt,
    });
  }

  return [...merged.values()].sort((left, right) => left.displayName.localeCompare(right.displayName));
}

export function DatabaseScopeProvider(props: PropsWithChildren) {
  const workspacesQuery = useWorkspaceSummaries();
  const derivedDatabases = useMemo(
    () => deriveDatabases(workspacesQuery.data ?? []),
    [workspacesQuery.data],
  );
  const [savedDatabases, setSavedDatabases] = useState(readSavedDatabases);
  const [selectedDatabaseIdState, setSelectedDatabaseIdState] = useState(readSelectedDatabaseId);
  const [isOpenDatabaseDialogOpen, setOpenDatabaseDialogOpen] = useState(false);

  const databases = useMemo(
    () => mergeDatabases(derivedDatabases, savedDatabases),
    [derivedDatabases, savedDatabases],
  );
  const selectedDatabase = useMemo<AFSDatabaseScopeRecord | null>(() => {
    if (selectedDatabaseIdState != null) {
      const matching = databases.find((item) => item.id === selectedDatabaseIdState);
      if (matching != null) {
        return matching;
      }
    }

    if (databases.length === 0) {
      return null;
    }

    return databases[0];
  }, [databases, selectedDatabaseIdState]);
  const selectedDatabaseId = selectedDatabase == null ? null : selectedDatabase.id;

  useEffect(() => {
    localStorage.setItem(SAVED_DATABASES_STORAGE_KEY, JSON.stringify(savedDatabases));
  }, [savedDatabases]);

  useEffect(() => {
    if (selectedDatabaseId === null) {
      localStorage.removeItem(SELECTED_DATABASE_STORAGE_KEY);
      return;
    }

    localStorage.setItem(SELECTED_DATABASE_STORAGE_KEY, selectedDatabaseId);
  }, [selectedDatabaseId]);

  useEffect(() => {
    if (selectedDatabaseIdState == null && databases.length > 0) {
      setSelectedDatabaseIdState(databases[0].id);
      return;
    }

    if (
      selectedDatabaseIdState != null &&
      !databases.some((item) => item.id === selectedDatabaseIdState) &&
      databases.length > 0
    ) {
      setSelectedDatabaseIdState(databases[0].id);
    }
  }, [databases, selectedDatabaseIdState]);

  const value = useMemo<DatabaseScopeContextValue>(
    () => ({
      clientMode: getAFSClientMode(),
      databases,
      selectedDatabase,
      selectedDatabaseId,
      isLoading: workspacesQuery.isLoading,
      selectDatabase: (databaseId: string) => {
        setSelectedDatabaseIdState(databaseId);
      },
      openDatabase: (input: OpenDatabaseInput) => {
        const databaseName = input.databaseName.trim();
        const displayName = input.displayName.trim() || databaseName;
        const id = slugify(databaseName === "" ? displayName : databaseName);
        const now = new Date().toISOString();

        setSavedDatabases((current) => {
          const next = current.filter((item) => item.id !== id);
          next.unshift({
            id,
            displayName,
            databaseName,
            endpointLabel: input.endpointLabel.trim(),
            dbIndex: input.dbIndex.trim(),
            lastOpenedAt: now,
          });
          return next;
        });
        setSelectedDatabaseIdState(id);
        setOpenDatabaseDialogOpen(false);
      },
      isOpenDatabaseDialogOpen,
      setOpenDatabaseDialogOpen,
    }),
    [databases, isOpenDatabaseDialogOpen, selectedDatabase, selectedDatabaseId, workspacesQuery.isLoading],
  );

  return (
    <DatabaseScopeContext.Provider value={value}>
      {props.children}
    </DatabaseScopeContext.Provider>
  );
}

export function useDatabaseScope() {
  const context = useContext(DatabaseScopeContext);
  if (context == null) {
    throw new Error("useDatabaseScope must be used inside DatabaseScopeProvider.");
  }

  return context;
}

export function useScopedWorkspaceSummaries() {
  const query = useWorkspaceSummaries();
  const { selectedDatabaseId } = useDatabaseScope();
  const data = useMemo(() => {
    const rows = query.data ?? [];
    if (selectedDatabaseId == null) {
      return rows;
    }
    return rows.filter((row) => row.databaseId === selectedDatabaseId);
  }, [query.data, selectedDatabaseId]);

  return {
    ...query,
    data,
  };
}

export function useScopedActivity(limit = 50) {
  const activityQuery = useActivity(limit);
  const workspacesQuery = useWorkspaceSummaries();
  const { selectedDatabaseId } = useDatabaseScope();

  const scopedActivity = useMemo(() => {
    const events = activityQuery.data ?? [];
    if (selectedDatabaseId == null) {
      return events;
    }

    const workspaceIds = new Set(
      (workspacesQuery.data ?? [])
        .filter((workspace) => workspace.databaseId === selectedDatabaseId)
        .map((workspace) => workspace.id),
    );

    return events.filter((event) => event.workspaceId != null && workspaceIds.has(event.workspaceId));
  }, [activityQuery.data, selectedDatabaseId, workspacesQuery.data]);

  return {
    ...activityQuery,
    data: scopedActivity,
    isLoading: activityQuery.isLoading || workspacesQuery.isLoading,
    isError: activityQuery.isError || workspacesQuery.isError,
  };
}
