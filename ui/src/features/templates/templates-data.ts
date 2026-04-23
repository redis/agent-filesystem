import type { IconType } from "@redis-ui/icons";
import {
  BotIcon,
  BookOpenIcon,
  FoldersIcon,
  SparklesIcon,
} from "../../components/lucide-icons";
import type { AFSMCPProfile } from "../../foundation/types/afs";

export type TemplateSeedFile = {
  path: string;
  content: string;
};

export type Template = {
  id: string;
  slug: string;
  title: string;
  tagline: string;
  icon: IconType;
  accent: string;
  profile: AFSMCPProfile;
  profileLabel: string;
  summary: readonly string[];
  whyItMatters: string;
  seedFiles: readonly TemplateSeedFile[];
  firstPrompt: string;
};

/* -------------------------------------------------------------------------- */
/* Shared Agent Memory                                                        */
/* -------------------------------------------------------------------------- */

const sharedMemoryFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Shared Agent Memory

A shared long-term memory for agents across your team. Every agent that
connects (Claude Code, Codex, Cursor, or any MCP client) reads from and
writes to the same memory, backed by Redis through Agent Filesystem.

## Layout

- \`shared-memory/index.md\` — curated rollup of all learnings, newest first.
- \`shared-memory/entries/YYYY-MM-DD-<slug>.md\` — one file per learning.
- \`AGENTS.md\` — the protocol every agent should follow when using this workspace.

## Why it's interesting

Redis sits behind the workspace, so reads are sub-millisecond and every
agent connected to this workspace sees writes immediately. Nothing to
sync, nothing to pull — just a filesystem that happens to be shared.

## Getting started

See \`AGENTS.md\` for the read/write protocol agents should follow.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This is a shared memory workspace. Multiple agents, potentially across
multiple machines and users, read and write the same files here.

## Before answering a non-trivial question

1. \`file_grep\` the \`shared-memory/\` tree for any of the key nouns in
   the user's question. If you find relevant entries, cite them in your
   answer and build on them rather than re-deriving the answer.
2. If nothing relevant exists, proceed normally.

## After discovering something durable

A "durable" learning is a non-obvious fact about this codebase, team,
or domain that will still be true next week and that another agent
could reuse. Debugging breadcrumbs and session-specific context do
**not** qualify.

When you find one:

1. Create \`shared-memory/entries/YYYY-MM-DD-<short-slug>.md\` using
   the template below.
2. Append a one-line entry to \`shared-memory/index.md\` under the most
   recent date heading. Keep it under 140 characters.

## Entry template

    ---
    date: YYYY-MM-DD
    agent: <your client, e.g. claude-code or codex>
    ---

    # <Title — a clear, concrete statement>

    **Context.** When or where does this apply?

    **Finding.** What is the durable fact?

    **Sources.** Files, links, or conversations the finding came from.

## Rules that keep concurrent writes safe

- **Never overwrite another agent's entry.** Always write new files,
  never edit existing entries.
- **\`index.md\` is append-only.** Add new lines under the newest date
  heading. Do not rewrite older lines.
- **Slugs must be unique.** Include a short random suffix if in doubt:
  \`2026-04-22-redis-ttl-3f9a.md\`.
`,
  },
  {
    path: "shared-memory/index.md",
    content: `# Memory index

Append new entries below the newest date heading. Keep each line under
140 characters — title plus a one-line summary and a link to the full
entry file.

## 2026-04-22

- [Example learning](entries/2026-04-22-example.md) — the first entry that shipped with this template; explains the expected shape.
`,
  },
  {
    path: "shared-memory/entries/2026-04-22-example.md",
    content: `---
date: 2026-04-22
agent: template
---

# Example memory entry

**Context.** This entry shipped with the Shared Agent Memory template
so the layout is visible to the next agent that connects.

**Finding.** Useful memory entries describe a durable fact plus where
the fact came from. They are not debugging trails or session recaps.
See \`AGENTS.md\` in the workspace root for the full protocol.

**Sources.** The Shared Agent Memory template in the AFS web UI.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* Org Coding Standards                                                       */
/* -------------------------------------------------------------------------- */

const standardsFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Org Coding Standards

A **read-only** source of truth for your team's coding standards.
Every developer's agents mount this workspace and consult it before
writing code. Updates flow through a small set of maintainers.

## Layout

- \`AGENTS.md\` — the protocol agents follow.
- \`standards/languages/<lang>.md\` — per-language rules.
- \`standards/review-checklist.md\` — what to check in every PR.
- \`standards/security.md\` — security rules.
- \`standards/architecture-principles.md\` — org-wide architecture defaults.

## Sharing this workspace

The MCP access token created alongside this workspace has the
\`workspace-ro\` profile. Agents can read the standards but cannot
modify them.

Distribute the token through your team password manager. Every
developer drops it into their MCP client config (see the dialog that
opened when you created this workspace) and their agent immediately
sees the same rules.

## Updating the standards

Edit the files through the AFS web UI, or create a second
\`workspace-rw\` token scoped to maintainers only. The next read from
any developer's agent picks up the change — no redeploy, no cache.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This workspace contains your organization's coding standards. It is
**read-only** for agents. Your role is to consult these files before
writing code and to cite them in reviews.

## Before writing or modifying code

1. Identify the language and surface you're about to touch.
2. \`file_read\` the matching file under \`standards/languages/\`.
3. \`file_read\` \`standards/architecture-principles.md\` and
   \`standards/security.md\` when relevant.
4. Apply the rules you read. If any rule conflicts with the user's
   request, surface the conflict and ask how to proceed — do not
   silently override the standard.

## When reviewing a PR or diff

1. \`file_read\` \`standards/review-checklist.md\`.
2. Walk each item against the diff. Cite the specific standard file
   and section when you flag something.

## If a standard seems wrong or missing

Do not edit it — this workspace is read-only. Instead, raise it with
the user and point at the specific file and line.
`,
  },
  {
    path: "standards/languages/go.md",
    content: `# Go

Replace this file with your team's Go standards. The template seeds a
minimal starting point.

## Structure

- Follow \`gofmt\` and \`goimports\` output. Do not hand-wrap.
- One concept per package. Resist the urge to create a \`util\` package.

## Error handling

- Wrap errors with \`fmt.Errorf("context: %w", err)\` when crossing a
  layer boundary.
- Never ignore errors without an explicit comment.

## Tests

- Table-driven tests where there is more than one input shape.
- Prefer \`t.TempDir()\` over manual temp-file cleanup.
`,
  },
  {
    path: "standards/languages/typescript.md",
    content: `# TypeScript

Replace this file with your team's TypeScript standards.

## Types

- Prefer \`type\` aliases for unions and discriminated unions, \`interface\`
  for public object contracts that might be extended.
- Never use \`any\`. Use \`unknown\` at boundaries and narrow.

## React components

- Components are functions. No classes.
- Keep prop types colocated with the component file.
- State flows down, events flow up. Avoid shared mutable singletons.

## Async

- Prefer \`async/await\` over raw promise chains.
- Always handle rejection — either \`try/catch\` or route through a
  boundary that does.
`,
  },
  {
    path: "standards/review-checklist.md",
    content: `# Review checklist

For every PR, check each item and cite the specific standard when you
flag something.

- [ ] Scope matches the PR description; no drive-by changes.
- [ ] New code follows the relevant \`standards/languages/*.md\`.
- [ ] No new \`any\`, \`interface{}\`, \`// @ts-ignore\`, or \`//nolint\`
      without a comment explaining why.
- [ ] Tests cover the new behavior and at least one failure mode.
- [ ] Error messages are specific and actionable.
- [ ] No secrets, tokens, or internal URLs in code or comments.
- [ ] Public API changes are documented.
- [ ] Dependencies added are justified; no drive-by adds.
`,
  },
  {
    path: "standards/security.md",
    content: `# Security standards

Replace with your team's rules. Defaults below.

## Secrets

- Never commit secrets. Use the org secret manager.
- Tokens in logs must be redacted.

## Input validation

- Validate all inputs at trust boundaries (HTTP handlers, CLI args,
  external API responses).
- Parameterize all SQL; never string-concatenate queries.

## Authorization

- Default deny. Explicitly allow per endpoint and per action.
- Never trust client-supplied user identity.
`,
  },
  {
    path: "standards/architecture-principles.md",
    content: `# Architecture principles

Replace with your team's principles. Defaults below.

## Simplicity first

- Prefer the simplest thing that works. Abstractions earn their way in
  by being demanded a second time, not predicted.
- Delete unused code. Do not keep it "just in case".

## Observability

- Log at boundaries with structured fields. Log IDs, not payloads.
- Metrics count things that matter to a human, not every function call.

## Failure

- Make failures explicit at boundaries. Don't swallow errors inside
  helpers that can't recover.
- Retries and timeouts are features. Design them in, don't add them after.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* Team Planning Board                                                        */
/* -------------------------------------------------------------------------- */

const teamBoardFiles: TemplateSeedFile[] = [
  {
    path: "README.md",
    content: `# Team Planning Board

A shared whiteboard for a team of humans and agents coordinating on a
project. One spec, one roadmap, a live view of who is working on what,
a place to record completed work, and a place to surface open questions.

## Layout

- \`plan/spec.md\` — the overall goal, scope, non-goals.
- \`plan/roadmap.md\` — phases and milestones.
- \`tasks/backlog.md\` — unstarted work.
- \`tasks/in-progress/<owner>-<slug>.md\` — one file per active task.
- \`tasks/done/<slug>.md\` — completed tasks, appended after finish.
- \`questions/<slug>.md\` — open decisions awaiting an answer.

## How it stays sane with many writers

- **Per-owner subdirs for drafts.** Each person writes under a file
  named for their handle, so two agents never edit the same file.
- **Append-only for shared docs.** \`spec.md\`, \`roadmap.md\`, and
  \`backlog.md\` grow by addition. Use owner markers
  (\`<!-- @handle 2026-04-22 -->\`) when amending.
- **Checkpoints per milestone.** This workspace's MCP profile includes
  \`checkpoint_create\`. Snapshot the board at each milestone so the
  timeline is recoverable.

See \`AGENTS.md\` for the full protocol.
`,
  },
  {
    path: "AGENTS.md",
    content: `# Protocol for this workspace

This workspace coordinates a team. Multiple agents and humans write
here. Follow these rules so you do not step on each other.

## Your handle

On first interaction, ask the user for a short stable handle (for
example \`alice\` or \`bob\`). Remember it for the session and use it in
every task file you create or update.

## Claiming a task

1. Look in \`tasks/backlog.md\` for a line that matches the user's goal.
2. \`file_write\` \`tasks/in-progress/<handle>-<slug>.md\` with:

       ---
       owner: <handle>
       started: YYYY-MM-DD
       progress: 0%
       status: active
       ---

       # <Task title>

       **Goal.** One paragraph.

       **Plan.** Bulleted steps.

       **Log.** Append dated bullets as you work.

3. Remove the matching line from \`tasks/backlog.md\` (edit the file,
   delete only that line).

## Making progress

Append a dated bullet to the **Log** section of your task file after
each meaningful step. Update the \`progress:\` front-matter field.

## Finishing a task

1. Move the content to \`tasks/done/<slug>.md\`.
2. Delete the in-progress file with \`file_delete_lines\` + \`file_write\`
   replacement, or simply \`file_write\` to an empty string.
3. If \`checkpoint_create\` is available to you, snapshot the workspace
   with a name like \`milestone-<slug>\`.

## Recording a question

\`file_write\` \`questions/<slug>.md\` with:

    ---
    asked-by: <handle>
    date: YYYY-MM-DD
    status: open
    ---

    # <Question>

    Answer here once resolved.

## Never

- Never edit another owner's in-progress task file.
- Never rewrite \`spec.md\` or \`roadmap.md\` — append new sections.
`,
  },
  {
    path: "plan/spec.md",
    content: `# Project spec

Replace the sample text with your real spec. Keep this file
append-only — add new sections rather than rewriting existing ones,
and use owner markers when amending.

## Goal

<What are we building, in one sentence?>

## Scope

- <In-scope item 1>
- <In-scope item 2>

## Non-goals

- <Out-of-scope item 1>

## Success criteria

- <Measurable outcome 1>
- <Measurable outcome 2>

<!-- @handle YYYY-MM-DD - example amend marker -->
`,
  },
  {
    path: "plan/roadmap.md",
    content: `# Roadmap

Phases and milestones. Snapshot the workspace (\`checkpoint_create\`)
each time you finish a milestone so the timeline is recoverable.

## Phase 1 — <name>

- **Milestone 1.1** <description> — target YYYY-MM-DD
- **Milestone 1.2** <description> — target YYYY-MM-DD

## Phase 2 — <name>

- **Milestone 2.1** <description> — target YYYY-MM-DD
`,
  },
  {
    path: "tasks/backlog.md",
    content: `# Backlog

Unstarted work, one line per task. When you claim a task, delete the
line and create \`tasks/in-progress/<handle>-<slug>.md\`.

- Example task — short description of the work to do.
- Another example task — different shape of work.
`,
  },
  {
    path: "tasks/in-progress/README.md",
    content: `# In-progress tasks

One file per active task, named \`<owner-handle>-<slug>.md\`.
See \`AGENTS.md\` in the workspace root for the task file format.
`,
  },
  {
    path: "tasks/done/README.md",
    content: `# Completed tasks

One file per completed task. Keep the history — do not delete files
from this directory. Past tasks are the easiest way for agents to
learn how your team approaches work.
`,
  },
  {
    path: "questions/README.md",
    content: `# Open questions

One file per question awaiting a decision. Close a question by
appending the answer and flipping \`status: open\` to \`status: resolved\`
in the front matter. Do not delete resolved questions — the history is
useful.
`,
  },
];

/* -------------------------------------------------------------------------- */
/* Template registry                                                          */
/* -------------------------------------------------------------------------- */

export const templates: readonly Template[] = [
  {
    id: "shared-agent-memory",
    slug: "shared-memory",
    title: "Shared Agent Memory",
    tagline:
      "A collective long-term memory your whole team of agents can read and write.",
    icon: BotIcon,
    accent: "#6366f1",
    profile: "workspace-rw",
    profileLabel: "Read / write",
    summary: [
      "One memory, many agents, any machine",
      "Append-only entries keep concurrent writes safe",
      "Works with Claude Code, Codex, Cursor, or any MCP client",
    ],
    whyItMatters:
      "Redis-backed filesystem means every agent sees learnings in real time, with sub-millisecond reads. Agent A's discovery from yesterday is Agent B's starting point today — no copy-paste, no syncing.",
    seedFiles: sharedMemoryFiles,
    firstPrompt:
      "What do you already know about this workspace? Grep shared-memory/ and summarize.",
  },
  {
    id: "org-coding-standards",
    slug: "coding-standards",
    title: "Org Coding Standards",
    tagline:
      "Read-only source of truth for your team's coding standards and review rules.",
    icon: BookOpenIcon,
    accent: "#0ea5e9",
    profile: "workspace-ro",
    profileLabel: "Read-only",
    summary: [
      "Every agent reads the same canonical standards",
      "Update once — every developer's agent sees it immediately",
      "Read-only MCP token means agents can't clobber the rules",
    ],
    whyItMatters:
      "Coding standards go stale the moment they live in a Notion page nobody reads. Here they live in a workspace your agents consult automatically, gated by a read-only MCP profile so nothing can edit them by accident.",
    seedFiles: standardsFiles,
    firstPrompt:
      "Summarize every standard in this workspace in one paragraph each.",
  },
  {
    id: "team-planning-board",
    slug: "team-board",
    title: "Team Planning Board",
    tagline:
      "Shared spec, in-flight tasks, completed work, and open questions — for humans and agents coordinating together.",
    icon: FoldersIcon,
    accent: "#22c55e",
    profile: "workspace-rw-checkpoint",
    profileLabel: "Read / write + checkpoints",
    summary: [
      "One workspace coordinates a whole team",
      "Per-owner subdirs avoid write conflicts",
      "Checkpoint at each milestone — full history recoverable",
    ],
    whyItMatters:
      "Kanban boards are great at state but terrible at context. This template keeps state (tasks, owners, progress) and context (spec, roadmap, open questions) in the same shared filesystem, navigable by every agent on your team.",
    seedFiles: teamBoardFiles,
    firstPrompt:
      "Read plan/spec.md and tasks/backlog.md, then help me claim the next task.",
  },
  {
    id: "blank",
    slug: "blank",
    title: "Blank Workspace",
    tagline: "Start empty and shape it as you go.",
    icon: SparklesIcon,
    accent: "#94a3b8",
    profile: "workspace-rw",
    profileLabel: "Read / write",
    summary: [
      "No seed files, no protocol — just a workspace",
      "Reach for this when your use case doesn't fit the others",
      "You can always add shape later",
    ],
    whyItMatters:
      "Sometimes you just want a fresh Redis-backed workspace to experiment in. Pick this, connect your agent, go.",
    seedFiles: [],
    firstPrompt: "What shall we build in this workspace?",
  },
] as const;

export function findTemplate(id: string): Template | undefined {
  return templates.find((template) => template.id === id);
}

/* -------------------------------------------------------------------------- */
/* Setup prompt generator                                                     */
/* -------------------------------------------------------------------------- */

export function buildSetupPrompt(template: Template, workspaceName: string) {
  if (template.seedFiles.length === 0) {
    return `You're connected to a fresh Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents. The workspace is currently empty.

Suggest three ways we could use this workspace based on what I'm working on. Wait for me to pick one before creating any files.`;
  }

  const intro = `I've connected you to an Agent Filesystem workspace named "${workspaceName}" via MCP. The file_read, file_write, file_list, file_grep, file_replace, file_insert, file_delete_lines, file_patch, file_lines, and file_glob tools reach its contents.

Initialize this workspace as "${template.title}" — ${template.tagline}

Use file_write to create each of the files below exactly as shown. The content for each file is the block between the opening "<<<FILE: path>>>" marker and the matching "<<<END>>>" marker.`;

  const fileBlocks = template.seedFiles
    .map(
      (file) => `<<<FILE: ${file.path}>>>
${file.content.trimEnd()}
<<<END>>>`,
    )
    .join("\n\n");

  const outro = `Once every file is written, run file_list on the workspace root and on each subdirectory you created, then give me a one-paragraph summary of the layout.

From then on, follow the protocol in AGENTS.md for this and every future session pointed at this workspace. When the user is ready, suggest they try:

> ${template.firstPrompt}`;

  return `${intro}

${fileBlocks}

${outro}`;
}
