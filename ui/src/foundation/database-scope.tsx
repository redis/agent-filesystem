import { useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { PropsWithChildren } from "react";
import { afsApi, getAFSClientMode } from "./api/afs";
import { useAuthSession } from "./auth-context";
import {
  useDatabases,
  useDeleteDatabaseMutation,
  useSaveDatabaseMutation,
  useSetDefaultDatabaseMutation,
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
  ownerSubject?: string;
  ownerLabel?: string;
  managementType?: string;
  purpose?: string;
  canEdit: boolean;
  canDelete: boolean;
  canCreateWorkspaces: boolean;
  endpointLabel: string;
  dbIndex: string;
  username: string;
  password: string;
  useTLS: boolean;
  isDefault: boolean;
  workspaceCount: number;
  activeSessionCount: number;
  isHealthy: boolean;
  connectionError?: string;
  lastWorkspaceRefreshAt?: string;
  lastWorkspaceRefreshError?: string;
  lastSessionReconcileAt?: string;
  lastSessionReconcileError?: string;
  // AFS aggregates
  afsTotalBytes: number;
  afsFileCount: number;
  // Redis server stats snapshot (from background poller); undefined until sampled
  stats?: {
    usedMemoryBytes: number;
    maxMemoryBytes: number;
    fragmentationRatio: number;
    keyCount: number;
    opsPerSec: number;
    cacheHitRate: number;
    connectedClients: number;
    sampledAt?: string;
  };
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
  unavailableDatabases: AFSDatabaseScopeRecord[];
  isLoading: boolean;
  errorMessage: string | null;
  saveDatabase: (input: SaveDatabaseInput) => Promise<void>;
  setDefaultDatabase: (databaseId: string) => Promise<void>;
  removeDatabase: (databaseId: string) => Promise<void>;
  reconcileCatalog: () => Promise<void>;
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
    ownerSubject: input.ownerSubject,
    ownerLabel: input.ownerLabel,
    managementType: input.managementType,
    purpose: input.purpose,
    canEdit: input.canEdit,
    canDelete: input.canDelete,
    canCreateWorkspaces: input.canCreateWorkspaces,
    endpointLabel: input.redisAddr,
    dbIndex: String(input.redisDB),
    username: input.redisUsername,
    password: input.redisPassword,
    useTLS: input.redisTLS,
    isDefault: input.isDefault,
    workspaceCount: input.workspaceCount,
    activeSessionCount: input.activeSessionCount,
    isHealthy: !input.connectionError,
    connectionError: input.connectionError,
    lastWorkspaceRefreshAt: input.lastWorkspaceRefreshAt,
    lastWorkspaceRefreshError: input.lastWorkspaceRefreshError,
    lastSessionReconcileAt: input.lastSessionReconcileAt,
    lastSessionReconcileError: input.lastSessionReconcileError,
    afsTotalBytes: input.afsTotalBytes,
    afsFileCount: input.afsFileCount,
    stats: input.stats,
  };
}

export function DatabaseScopeProvider(props: PropsWithChildren) {
  const clientMode = getAFSClientMode();
  const queryClient = useQueryClient();
  const auth = useAuthSession();
  const queriesEnabled = !auth.isLoading && (!auth.config.enabled || auth.isAuthenticated);
  const usesHostedAccountAuth = clientMode === "http" && auth.supportsAccountAuth;
  const databasesQuery = useDatabases(queriesEnabled);
  const saveDatabaseMutation = useSaveDatabaseMutation();
  const setDefaultDatabaseMutation = useSetDefaultDatabaseMutation();
  const deleteDatabaseMutation = useDeleteDatabaseMutation();
  const [legacyMigrated, setLegacyMigrated] = useState(clientMode !== "http");

  const databases = useMemo(
    () => (databasesQuery.data ?? []).map(mapDatabaseRecord),
    [databasesQuery.data],
  );
  const errorMessage = !queriesEnabled
    ? null
    : databasesQuery.error instanceof Error
    ? databasesQuery.error.message
    : databasesQuery.error != null
      ? "Unable to load databases."
      : null;
  const unavailableDatabases = useMemo(
    () => databases.filter((database) => !database.isHealthy),
    [databases],
  );

  useEffect(() => {
    if (usesHostedAccountAuth) {
      localStorage.removeItem(LEGACY_SAVED_DATABASES_STORAGE_KEY);
      if (!legacyMigrated) {
        setLegacyMigrated(true);
      }
      return;
    }

    if (clientMode !== "http" || legacyMigrated || databasesQuery.isLoading || !queriesEnabled) {
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
  }, [clientMode, databases, databasesQuery, legacyMigrated, queriesEnabled, usesHostedAccountAuth]);

  const value = useMemo<DatabaseScopeContextValue>(
    () => ({
      clientMode,
      databases,
      unavailableDatabases,
      isLoading: auth.isLoading || (queriesEnabled && databasesQuery.isLoading),
      errorMessage,
      saveDatabase: async (input: SaveDatabaseInput) => {
        await saveDatabaseMutation.mutateAsync(input);
      },
      setDefaultDatabase: async (databaseId: string) => {
        await setDefaultDatabaseMutation.mutateAsync(databaseId);
      },
      removeDatabase: async (databaseId: string) => {
        await deleteDatabaseMutation.mutateAsync(databaseId);
      },
      reconcileCatalog: async () => {
        await afsApi.reconcileCatalog();
        await queryClient.invalidateQueries({
          predicate: (query) => Array.isArray(query.queryKey) && query.queryKey[0] === "afs",
        });
      },
    }),
    [
      clientMode,
      databases,
      unavailableDatabases,
      auth.isLoading,
      databasesQuery.isLoading,
      errorMessage,
      deleteDatabaseMutation,
      queryClient,
      saveDatabaseMutation,
      queriesEnabled,
      setDefaultDatabaseMutation,
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
  const { databases } = useDatabaseScope();
  const hiddenDatabaseIDs = new Set(
    databases
      .filter((database) => !database.canCreateWorkspaces)
      .map((database) => database.id),
  );

  return {
    ...query,
    data: (query.data ?? []).filter((workspace) => !hiddenDatabaseIDs.has(workspace.databaseId)),
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
  const { databases } = useDatabaseScope();
  const hiddenDatabaseIDs = new Set(
    databases
      .filter((database) => !database.canCreateWorkspaces)
      .map((database) => database.id),
  );

  return {
    ...query,
    data: (query.data ?? []).filter((agent) => {
      if (!agent.databaseId) {
        return true;
      }
      return !hiddenDatabaseIDs.has(agent.databaseId);
    }),
  };
}
