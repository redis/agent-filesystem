// loaders — pure functions from (params) → data.
// today these read mock JSON. tomorrow they hit /v1/...
// the call sites should not change.

import workspacesData from './mocks/workspaces.json'
import checkpointsData from './mocks/checkpoints.json'
import sessionsData from './mocks/sessions.json'
import activityData from './mocks/activity.json'
import toolsData from './mocks/tools.json'
import receiptsData from './mocks/receipts.json'
import whyData from './mocks/why.json'
import type { Workspace, Checkpoint, Session, ActivityEvent, Tool, Receipt, RejectionReason } from './types'

export const loadWorkspaces = () => workspacesData as {
  items: Workspace[]
  etag: string
  count: number
  dirty_count: number
}

export const loadWorkspace = (id: string): Workspace | null => {
  return (workspacesData.items as Workspace[]).find((w) => w.id === id) ?? null
}

export const loadCheckpoints = (workspaceId?: string): Checkpoint[] => {
  const all = checkpointsData.items as Checkpoint[]
  return workspaceId ? all.filter((c) => c.workspace_id === workspaceId) : all
}

export const loadCheckpoint = (id: string): Checkpoint | null => {
  return (checkpointsData.items as Checkpoint[]).find((c) => c.id === id) ?? null
}

export const loadSessions = (workspaceId?: string): Session[] => {
  const all = sessionsData.items as Session[]
  return workspaceId ? all.filter((s) => s.workspace_id === workspaceId) : all
}

export const loadActivity = (filter?: {
  workspace?: string
  agent?: string
  session?: string
  since?: string
}): ActivityEvent[] => {
  let items = activityData.items as ActivityEvent[]
  if (filter?.workspace) items = items.filter((e) => e.workspace_id === filter.workspace)
  if (filter?.agent) items = items.filter((e) => e.agent_id === filter.agent)
  if (filter?.session) items = items.filter((e) => e.session_id === filter.session)
  if (filter?.since) items = items.filter((e) => e.ts >= filter.since!)
  return items
}

export const loadTools = (filter?: { id?: string; family?: string; scope?: string }): Tool[] => {
  let items = toolsData.items as Tool[]
  if (filter?.id) items = items.filter((t) => t.id === filter.id)
  if (filter?.family) items = items.filter((t) => t.family === filter.family)
  if (filter?.scope) items = items.filter((t) => t.scope === filter.scope)
  return items
}

export const loadTool = (id: string): Tool | null => {
  return (toolsData.items as Tool[]).find((t) => t.id === id) ?? null
}

export const loadReceipt = (hash: string): Receipt | null => {
  return (receiptsData.items as Receipt[]).find((r) => r.hash.startsWith(hash) || r.hash === hash) ?? null
}

export const loadReceipts = (): Receipt[] => receiptsData.items as Receipt[]

export const loadWhy = (actionId: string): RejectionReason | null => {
  return (whyData.items as RejectionReason[]).find((w) => w.action_id === actionId) ?? null
}

export const loadAllWhy = (): RejectionReason[] => whyData.items as RejectionReason[]

// agent identity for this prototype. in real life this comes from the bearer token.
export const currentCapability = () => ({
  token: 'tok_demo_workspace_rw',
  scope: 'workspace' as const,
  workspace_id: 'payments-portal',
  profile: 'workspace-rw',
  readonly: false,
  expires_at: '2026-05-08T00:00:00Z',
  granted: ['workspace_list', 'workspace_create', 'workspace_fork', 'file_read', 'file_write', 'file_grep', 'file_glob', 'file_replace', 'file_patch', 'afs_status', 'checkpoint_create'],
})
