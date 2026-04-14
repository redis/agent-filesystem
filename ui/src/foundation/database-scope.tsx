import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { PropsWithChildren } from "react";
import { afsApi, getAFSClientMode } from "./api/afs";
import {
  useDatabases,
  useDeleteDatabaseMutation,
  useSaveDatabaseMutation,
  useActivity,
  useAgents,
  useWorkspaceSummaries,
} from "./hooks/use-afs";
import type { AFSClientMode, SaveDatabaseInput } from "./types/afs";

const LEGACY_SAVED_DATABASES_STORAGE_KEY = "afs_saved_databases_v1";

export type AFSDatabaseScopeRecord = {
  id: string;
  displayName: string;
  databaseName: string;
  description: string;
  endpointLabel: string;
  dbIndex: string;
  username: string;
  password: string;
  useTLS: boolean;
  workspaceCount: number;
  connectionError?: string;
};

type LegacySavedDatabaseRecord = {
  id: string;
  displayName: string;
  databaseName: string;
  description: string;
  endpointLabel: string;
  dbIndex: string;
  username: string;
  password: string;
  hidden?: boolean;
};

type DatabaseScopeContextValue = {
  clientMode: AFSClientMode;
  databases: AFSDatabaseScopeRecord[];
  isLoading: boolean;
  saveDatabase: (input: SaveDatabaseInput) => Promise<void>;
  removeDatabase: (databaseId: string) => Promise<void>;
};

const DatabaseScopeContext = createContext<DatabaseScopeContextValue | null>(null);

function readLegacySavedDatabases(): LegacySavedDatabaseRecord[] {
  const raw = localStorage.getItem(LEGACY_SAVED_DATABASES_STORAGE_KEY);
  if (raw == null) {
    return [];
  }

  try {
    return (JSON.parse(raw) as LegacySavedDatabaseRecord[]).filter((record) => !record.hidden);
  } catch {
    return [];
  }
}

function looksLikeRedisAddress(value: string) {
  const trimmed = value.trim().replace(/^rediss?:\/\//, "");
  return /^[^:\s]+:\d+$/.test(trimmed);
}

function mapDatabaseRecord(input: Awaited<ReturnType<typeof afsApi.listDatabases>>[number]): AFSDatabaseScopeRecord {
  return {
    id: input.id,
    displayName: input.name,
    databaseName: input.name,
    description: input.description,
    endpointLabel: input.redisAddr,
    dbIndex: String(input.redisDB),
    username: input.redisUsername,
    password: input.redisPassword,
    useTLS: input.redisTLS,
    workspaceCount: input.workspaceCount,
    connectionError: input.connectionError,
  };
}

export function DatabaseScopeProvider(props: PropsWithChildren) {
  const clientMode = getAFSClientMode();
  const databasesQuery = useDatabases();
  const saveDatabaseMutation = useSaveDatabaseMutation();
  const deleteDatabaseMutation = useDeleteDatabaseMutation();
  const [legacyMigrated, setLegacyMigrated] = useState(clientMode !== "http");

  const databases = useMemo(
    () => (databasesQuery.data ?? []).map(mapDatabaseRecord),
    [databasesQuery.data],
  );

  useEffect(() => {
    if (clientMode !== "http" || legacyMigrated || databasesQuery.isLoading) {
      return;
    }

    const legacy = readLegacySavedDatabases()
      .filter((record) => looksLikeRedisAddress(record.endpointLabel || record.databaseName));
    if (legacy.length === 0) {
      setLegacyMigrated(true);
      localStorage.removeItem(LEGACY_SAVED_DATABASES_STORAGE_KEY);
      return;
    }

    let cancelled = false;
    void (async () => {
      for (const record of legacy) {
        if (cancelled) {
          return;
        }
        if (databases.some((item) => item.id === record.id)) {
          continue;
        }
        await afsApi.saveDatabase({
          id: record.id,
          name: record.displayName || record.databaseName,
          description: record.description || "",
          redisAddr: record.endpointLabel || record.databaseName,
          redisUsername: record.username || "",
          redisPassword: record.password || "",
          redisDB: Number.parseInt(record.dbIndex || "0", 10) || 0,
          redisTLS: record.endpointLabel.startsWith("rediss://"),
        });
      }
      if (!cancelled) {
        localStorage.removeItem(LEGACY_SAVED_DATABASES_STORAGE_KEY);
        setLegacyMigrated(true);
        await databasesQuery.refetch();
      }
    })().catch(() => {
      if (!cancelled) {
        setLegacyMigrated(true);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [clientMode, databases, databasesQuery, legacyMigrated]);

  const value = useMemo<DatabaseScopeContextValue>(
    () => ({
      clientMode,
      databases,
      isLoading: databasesQuery.isLoading,
      saveDatabase: async (input: SaveDatabaseInput) => {
        await saveDatabaseMutation.mutateAsync(input);
      },
      removeDatabase: async (databaseId: string) => {
        await deleteDatabaseMutation.mutateAsync(databaseId);
      },
    }),
    [
      clientMode,
      databases,
      databasesQuery.isLoading,
      deleteDatabaseMutation,
      saveDatabaseMutation,
    ],
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
  const query = useWorkspaceSummaries(null);

  return {
    ...query,
    data: query.data ?? [],
  };
}

export function useScopedActivity(limit = 50) {
  const query = useActivity(null, limit);

  return {
    ...query,
    data: query.data ?? [],
  };
}

export function useScopedAgents() {
  const query = useAgents(null);

  return {
    ...query,
    data: query.data ?? [],
  };
}
