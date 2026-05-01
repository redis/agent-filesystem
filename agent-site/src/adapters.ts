// adapters.ts — translate raw control-plane responses into the agent-site's
// internal types. one place for the impedance match.
//
// the control plane today doesn't emit `_actions` (HATEOAS) or top-level
// `etag`, so we synthesize both client-side from (resource, capability).

import type { Action, Checkpoint, Session, Workspace } from './types'

// ────────────────────────────────────────────── raw shapes (subset)

type RawWorkspace = {
  id: string
  name: string
  database_id: string
  database_name?: string
  status: string
  draft_state: string
  file_count: number
  folder_count: number
  total_bytes: number
  checkpoint_count: number
  last_checkpoint_at: string | null
  updated_at: string
  region?: string
  source?: string
  description?: string
  head_checkpoint_id?: string
  tags?: string[]
}

type RawCheckpoint = {
  id: string
  name: string
  author?: string
  description?: string | null
  note?: string | null
  kind?: string
  source?: string
  parent_checkpoint_id?: string | null
  manifest_hash: string
  created_at: string
  file_count: number
  folder_count: number
  total_bytes: number
  is_head?: boolean
}

type RawSession = {
  id: string
  agent_id?: string
  workspace_id?: string
  label?: string
  client_label?: string
  client?: string
  state?: string
  started_at?: string
  last_seen?: string
  last_heartbeat_at?: string
  ops_count?: number
}

// ────────────────────────────────────────────── workspace

export function adaptWorkspace(raw: RawWorkspace): Workspace {
  return {
    id: raw.id,
    name: raw.name,
    database_id: raw.database_id,
    status: normalizeStatus(raw.status),
    draft_state: raw.draft_state === 'dirty' ? 'dirty' : 'clean',
    file_count: raw.file_count,
    folder_count: raw.folder_count,
    total_bytes: raw.total_bytes,
    checkpoint_count: raw.checkpoint_count,
    last_checkpoint_at: raw.last_checkpoint_at,
    updated_at: raw.updated_at,
    etag: `w/"${raw.id}-${raw.updated_at}"`,
    _actions: workspaceActions(raw),
  }
}

export function adaptWorkspaceList(raw: { items: RawWorkspace[] }): {
  items: Workspace[]
  etag: string
  count: number
  dirty_count: number
} {
  const items = (raw.items || []).map(adaptWorkspace)
  return {
    items,
    etag: `w/"list-${items.length}-${new Date().toISOString().slice(0, 19)}"`,
    count: items.length,
    dirty_count: items.filter((w) => w.draft_state === 'dirty').length,
  }
}

function normalizeStatus(s: string): 'healthy' | 'degraded' | 'offline' {
  if (s === 'healthy') return 'healthy'
  if (s === 'offline') return 'offline'
  return 'degraded'
}

function workspaceActions(w: RawWorkspace): Action[] {
  const id = w.id
  const ref = w.name || w.id   // CLI prefers the friendly name
  // ordered: featured first (shown inline), rest behind "more"
  return [
    { verb: 'checkpoint', cli: ['afs', 'cp', 'create',  ref, '<checkpoint-name>'],  href: `/workspaces/${id}/checkpoints`, method: 'POST',   idempotent: true,  schema: { name: 'string', note: 'string?' } },
    { verb: 'restore',    cli: ['afs', 'cp', 'restore', ref, '<checkpoint-id>'],    href: `/workspaces/${id}/restore`,     method: 'POST',   idempotent: false, schema: { checkpoint_id: 'string' } },
    { verb: 'diff',       cli: ['afs', 'cp', 'diff',    ref, '<base>', '<head>'],   href: `/workspaces/${id}/diff`,        method: 'GET',    idempotent: true,  schema: { base: 'ref', head: 'ref' } },
    { verb: 'fork',       cli: ['afs', 'ws', 'fork',    ref, '<new-name>'],         href: `/workspaces/${id}/fork`,        method: 'POST',   idempotent: false, schema: { name: 'string' } },
    { verb: 'delete',     cli: ['afs', 'ws', 'delete',  ref],                       href: `/workspaces/${id}`,             method: 'DELETE', idempotent: false },
  ]
}

// ────────────────────────────────────────────── checkpoint

export function adaptCheckpoint(raw: RawCheckpoint, workspaceId: string): Checkpoint {
  return {
    id: raw.id,
    workspace_id: workspaceId,
    name: raw.name,
    parent_id: raw.parent_checkpoint_id ?? null,
    created_at: raw.created_at,
    author: raw.author ?? 'unknown',
    source: (raw.source as Checkpoint['source']) ?? 'mcp',
    manifest_hash: raw.manifest_hash.startsWith('sha256:') ? raw.manifest_hash : `sha256:${raw.manifest_hash}`,
    file_count: raw.file_count,
    folder_count: raw.folder_count,
    total_bytes: raw.total_bytes,
    delta_files: 0,   // not surfaced by current api
    delta_bytes: 0,
    note: raw.note ?? raw.description ?? null,
    _actions: [
      // featured first (shown inline), rest behind "more"
      { verb: 'restore',      cli: ['afs', 'cp', 'restore', workspaceId, raw.id],         href: `/workspaces/${workspaceId}/restore`, method: 'POST', idempotent: false, schema: { checkpoint_id: raw.id } },
      { verb: 'diff-vs-head', cli: ['afs', 'cp', 'diff',    workspaceId, raw.id, 'working-copy'], href: `/workspaces/${workspaceId}/diff?base=checkpoint:${raw.id}&head=working-copy`, method: 'GET', idempotent: true },
      { verb: 'fork-from',    cli: ['afs', 'ws', 'fork',    workspaceId, '<new-name>', '--from', raw.id], href: `/workspaces/${workspaceId}/fork`, method: 'POST', idempotent: false, schema: { name: 'string', from_checkpoint: raw.id } },
    ],
  }
}

// ────────────────────────────────────────────── session

export function adaptSession(raw: RawSession, fallbackWorkspaceId?: string): Session {
  const id = raw.id
  return {
    id,
    agent_id: raw.agent_id ?? 'unknown',
    workspace_id: raw.workspace_id ?? fallbackWorkspaceId ?? '',
    label: raw.label ?? raw.client_label ?? '',
    started_at: raw.started_at ?? raw.last_seen ?? raw.last_heartbeat_at ?? new Date().toISOString(),
    last_seen: raw.last_seen ?? raw.last_heartbeat_at ?? new Date().toISOString(),
    client: raw.client ?? raw.client_label ?? 'unknown',
    state: (raw.state as Session['state']) ?? 'active',
    ops_count: raw.ops_count ?? 0,
    _actions: [
      { verb: 'kill', cli: ['afs', 'session', 'kill', id],   href: `/sessions/${id}`,         method: 'DELETE', idempotent: true },
      { verb: 'tail', cli: ['afs', 'log', '--follow', '--session', id], href: `/activity?session=${id}`, method: 'GET',    idempotent: true },
    ],
  }
}

// raw single-workspace detail response (with embedded checkpoints)
export type RawWorkspaceDetail = RawWorkspace & {
  checkpoints?: RawCheckpoint[]
  sessions?: RawSession[]
}

export function adaptWorkspaceDetail(raw: RawWorkspaceDetail): {
  workspace: Workspace
  checkpoints: Checkpoint[]
  sessions: Session[]
} {
  const workspace = adaptWorkspace(raw)
  return {
    workspace,
    checkpoints: (raw.checkpoints ?? []).map((c) => adaptCheckpoint(c, raw.id)),
    sessions: (raw.sessions ?? []).map((s) => adaptSession(s, raw.id)),
  }
}
