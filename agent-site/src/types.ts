// shared shapes. anemic on purpose — no methods, just records.

export type ISO = string

export type Workspace = {
  id: string
  name: string
  database_id: string
  status: 'healthy' | 'degraded' | 'offline'
  draft_state: 'clean' | 'dirty'
  file_count: number
  folder_count: number
  total_bytes: number
  checkpoint_count: number
  last_checkpoint_at: ISO | null
  updated_at: ISO
  etag: string
  _actions: Action[]
}

export type Checkpoint = {
  id: string
  workspace_id: string
  name: string
  parent_id: string | null
  created_at: ISO
  author: string
  source: 'agent_sync' | 'mcp' | 'cli' | 'import' | 'server_restore'
  manifest_hash: string
  file_count: number
  folder_count: number
  total_bytes: number
  delta_files: number
  delta_bytes: number
  note: string | null
  _actions: Action[]
}

export type Session = {
  id: string
  agent_id: string
  workspace_id: string
  label: string
  started_at: ISO
  last_seen: ISO
  client: string
  state: 'active' | 'idle' | 'closing'
  ops_count: number
  _actions: Action[]
}

export type ActivityEvent = {
  id: string
  ts: ISO
  workspace_id: string
  session_id: string | null
  agent_id: string | null
  user: string
  source: 'mcp' | 'cli' | 'agent_sync' | 'import' | 'checkpoint' | 'server_restore'
  op: 'file_write' | 'file_delete' | 'file_create' | 'file_replace' | 'file_patch' | 'checkpoint_create' | 'checkpoint_restore' | 'workspace_fork' | 'session_open' | 'session_close'
  path?: string
  bytes_delta?: number
  hash?: string
}

export type Receipt = {
  hash: string
  action_id: string
  ts: ISO
  verb: string
  resource: string
  agent_id: string
  session_id: string
  user: string
  before_ref: string | null
  after_ref: string
  bytes_delta: number
  cost: { tokens: number; bytes: number; ms: number }
  undo_token: string
  replay: { curl: string; cli: string; mcp: string }
  signature: string
}

export type Action = {
  verb: string
  href: string
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  idempotent: boolean
  cli?: string[]                       // canonical CLI form, e.g. ['afs', 'ws', 'fork', '<name>', '<new-name>']
  schema?: Record<string, string>
  description?: string
}

export type Tool = {
  id: string
  name: string
  surface: 'mcp' | 'http' | 'cli'
  family: string
  scope: 'read' | 'write' | 'admin'
  profile: 'workspace-ro' | 'workspace-rw' | 'workspace-rw-checkpoint' | 'admin-ro' | 'admin-rw'
  description: string
  params: { name: string; type: string; required: boolean; description?: string }[]
  returns: string
  example: { args: Record<string, unknown>; result: unknown }
}

export type Capability = {
  token: string
  scope: 'workspace' | 'control-plane'
  workspace_id: string | null
  profile: string
  readonly: boolean
  expires_at: ISO | null
  granted: string[]   // tool ids permitted
}

export type RejectionReason = {
  action_id: string
  ts: ISO
  rejected: true
  reason: string
  required_capability: string
  current_capability: string
  suggested_fix: string
  retry_after: number | null
  related_action: string | null
}

export type RequestShape = {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  path: string
  query?: Record<string, string>
  body?: unknown
  // headers we surface in the footer
  headers?: { etag?: string; cost_ms?: number; cost_bytes?: number; cost_tokens?: number; idempotency_key?: string }
}
