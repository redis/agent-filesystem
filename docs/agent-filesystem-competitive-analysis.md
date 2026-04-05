# Agent Filesystem Competitive Analysis

Date: 2026-04-02

## Executive Summary

The market is not really competing on "can an agent read and write files." That part is easy.

The real problem is:

1. Give an agent a workspace that feels like a normal filesystem and shell.
2. Keep that workspace safe, isolated, and reviewable.
3. Preserve state across long-running or multi-step work.
4. Make branching, rollback, and debugging cheap.
5. Let humans and multiple agents inspect or join the same session without chaos.

The current landscape falls into a few distinct patterns:

- Copy-on-write agent filesystems over a real repo, with strong auditability.
- Sandboxes or microVMs with their own real disk.
- Sandboxes plus persistent volumes and snapshots.
- Tool-level filesystem APIs exposed to agents through MCP or SDKs.
- Plain "just give the agent a bash shell in a repo" approaches.

`agent-filesystem` already covers important ground:

- shared durable state in Redis,
- filesystem semantics,
- multiple access surfaces (`FS.*`, Python, MCP, FUSE, NFS),
- inspectable storage,
- strong text-first affordances like `GREP` and BM25 search,
- easy cleanup and multi-client access.

But against the broader market problem, `agent-filesystem` is still incomplete:

- it is a storage layer more than a complete agent workspace product,
- it does not yet make snapshotting, branching, diffing, and replay first-class,
- it does not by itself solve safe execution,
- it is weaker than real filesystems for large-file and heavy build/test workloads,
- its current path-keyed model creates rename and range-I/O pressure that the backlog already recognizes.

My main conclusion:

`agent-filesystem` should not try to out-E2B or out-Daytona as a generic secure compute platform. Its best path is to become the best shared, queryable, branchable agent workspace layer, and then pair with shells, containers, or microVM sandboxes where needed.

The most important strategic move is to add an overlay/session model:

- read-only base tree,
- Redis-backed writable overlay,
- cheap snapshots and forks,
- durable audit log,
- `diff` and `apply` back to Git or a host directory.

That would let `agent-filesystem` compete much more directly with AgentFS while keeping its own advantages in distribution, shared state, and queryability.

## The Problem These Projects Are Actually Solving

The phrase "virtualizing the filesystem for agents" hides several different jobs to be done.

### Job 1: Safe execution

The agent needs somewhere to run untrusted code, shell commands, builds, tests, browsers, and package managers without damaging the host.

This pushes solutions toward:

- Docker-style sandboxes,
- nsjail,
- microVMs,
- Kubernetes sandbox controllers,
- hosted sandbox products.

### Job 2: Persistent workspace state

The agent needs files to survive beyond one prompt or one process:

- code changes,
- generated reports,
- caches,
- logs,
- memory files,
- session metadata.

This pushes solutions toward:

- durable volumes,
- snapshots,
- network filesystems,
- database-backed virtual filesystems,
- SQLite- or Redis-backed state stores.

### Job 3: Compatibility with existing tools

Agents are best when they can use normal things:

- `bash`,
- `git`,
- `pytest`,
- `npm`,
- `go test`,
- editors,
- compilers,
- linters.

This pushes solutions toward real POSIX filesystems or very convincing mounts.

### Job 4: Auditability and rollback

Developers want to know:

- what changed,
- what tool wrote it,
- what command produced it,
- what to revert,
- what to branch,
- what to replay.

This pushes solutions toward:

- Git-first workflows,
- copy-on-write overlays,
- event logs,
- SQLite histories,
- snapshots and restore points.

### Job 5: Multi-agent and human collaboration

The workspace should support:

- a second shell joining an active session,
- multiple agents sharing state,
- humans inspecting progress,
- controlled handoff between tasks.

This pushes solutions toward:

- shared session filesystems,
- networked state layers,
- attachable sandboxes,
- audit timelines and diffs.

## Evaluation Criteria

These are the dimensions that matter most when comparing approaches:

| Dimension | Why it matters |
|---|---|
| Isolation | Prevent host damage and cross-session contamination |
| POSIX compatibility | Lets agents use normal shells and build tools |
| Persistence | Survive across runs, sessions, or agent resumes |
| Branching and rollback | Cheap experimentation and recovery |
| Inspectability | Humans can see state, debug, and reason about it |
| Multi-client sharing | Humans and agents can attach to the same workspace |
| Performance at scale | Large repos, large files, many inodes, concurrent writes |
| Operational simplicity | How much infra and custom machinery is required |
| Queryability | Search, audit, analytics, and structured introspection |

## Implementation Patterns in the Market

### 1. Host-mounted repo plus sandboxed shell

Representative examples:

- GitHub Agentic Workflows AWF
- OpenHands Docker Runtime
- "just-bash in a container" setups

How it works:

- Mount a real host or checked-out repo into a container.
- Give the agent a shell and normal tools.
- Rely on the container boundary for some isolation.

Why people choose it:

- maximum compatibility,
- simplest mental model,
- native `git diff`,
- fastest path to usefulness.

Where it breaks:

- isolation can be partial,
- persistence and rollback are usually just "whatever the host filesystem and Git give you,"
- multi-agent collaboration is ad hoc,
- auditability is weak unless you add logs and snapshots separately.

### 2. Isolated sandbox with its own filesystem

Representative examples:

- E2B sandboxes
- Arrakis microVMs

How it works:

- Each agent gets a real Linux environment with its own disk.
- The filesystem is real, not emulated, but scoped to the sandbox.

Why people choose it:

- strong compatibility,
- strong safety,
- easy code execution,
- good fit for arbitrary build/test/browser workflows.

Where it breaks:

- state portability is not automatic,
- branching and diffing are often separate concerns,
- shared state across agents can be awkward,
- inspection may depend on platform APIs rather than the filesystem itself.

### 3. Sandboxes plus persistent volumes and snapshots

Representative examples:

- Daytona
- Modal
- Windmill AI sandbox
- Kubernetes agent-sandbox

How it works:

- Compute and storage are separate primitives.
- A sandbox runs code.
- A volume persists files across sessions.
- A snapshot or hibernation mechanism captures longer-lived state.

Why people choose it:

- good operational model for many agents,
- clear resource boundaries,
- strong platform story,
- long-running stateful agent workflows become practical.

Where it breaks:

- often heavier operationally,
- concurrency semantics can be weak or explicit "last write wins",
- Git-style review and diff are still separate problems,
- users still need a story for safe repo overlays and human review.

### 4. Agent-native overlay or copy-on-write filesystem

Representative example:

- AgentFS

How it works:

- Preserve a clean base tree.
- Run the agent against a writable overlay or copy-on-write session.
- Capture files, key-value state, and tool calls in one agent-centric storage system.

Why people choose it:

- directly addresses agent workflows,
- protects the source tree,
- makes rollback and session replay cheap,
- encourages auditability and session sharing.

Where it breaks:

- more custom machinery,
- POSIX edge cases are hard,
- build-heavy workloads still want a very real shell and kernel,
- large binary data and deep compatibility are still tricky.

### 5. Tool-level filesystem APIs

Representative examples:

- filesystem MCP servers
- `agent-filesystem` MCP mode

How it works:

- The agent does not get a real mounted filesystem.
- It gets structured tools like `read_file`, `write_file`, `ls`, `grep`, `mkdir`, and `rm`.

Why people choose it:

- safer,
- easier to confine,
- token-efficient for text workflows,
- easier to instrument.

Where it breaks:

- shell tools do not see the same world,
- many developer workflows assume a real filesystem,
- compatibility is much lower for build/test tasks.

## Project-by-Project Landscape

### AgentFS

What it is:

- Turso describes AgentFS as "the filesystem for agents."
- The GitHub repo describes four components: SDK, CLI, specification, and an experimental sandbox.
- It is explicitly SQLite-based and stores filesystem state, key-value state, and tool call history in a single agent-oriented database.
- Turso's docs position it as a copy-on-write environment for agents like Claude Code, Codex, and OpenCode, where the original tree remains untouched and a shell can join the same session.

What problem it is solving:

- protect the original repo,
- give each task an isolated session,
- make agent state portable and auditable,
- let humans and agents share a session.

Strengths:

- strongest framing around the actual agent workflow problem,
- built-in auditability,
- branchable, portable state,
- explicit session semantics.

Weaknesses:

- currently alpha,
- more specialized than a plain filesystem,
- likely less mature than mainstream sandboxes for arbitrary heavy compute.

Threat to `agent-filesystem`:

- This is the closest conceptual competitor.
- AgentFS is not just "storage for files"; it is a product thesis for agent workspaces.

Opportunity for `agent-filesystem`:

- Redis could provide better multi-client sharing, networked collaboration, and queryable operational state than a local SQLite-centric design.
- `agent-filesystem` can borrow the session and overlay model without copying the entire architecture.

### E2B

What it is:

- E2B describes itself as isolated sandboxes for agents to execute code, process data, and run tools.
- Its docs say each sandbox has its own isolated filesystem.
- E2B also has Volumes in private beta for persistent storage independent of sandbox lifecycle.
- The product site says sandboxes are powered by Firecracker microVMs.

What problem it is solving:

- secure code execution,
- agent compute with real tools,
- sandbox lifecycle management,
- optional durable storage.

Strengths:

- strong isolation,
- real Linux environment,
- excellent fit for code execution and tool use,
- clean developer SDK story.

Weaknesses:

- the filesystem itself is not the core innovation,
- branching, overlaying a repo, and durable audit trails are not the main abstraction,
- volumes are still a platform storage primitive rather than an agent-native workspace model.

Threat to `agent-filesystem`:

- If the user wants safe execution first, E2B is a stronger answer.

Opportunity for `agent-filesystem`:

- Pair with an E2B-like sandbox model instead of competing head-on.
- Let Redis back the workspace state, search, diffs, and collaboration layer.

### Daytona

What it is:

- Daytona positions itself as secure and elastic infrastructure for AI-generated code.
- The docs describe isolated sandboxes, S3-backed snapshots, persistent volumes, and a toolbox API for file, Git, process, and code execution operations.
- The GitHub repo emphasizes persistence, file/Git APIs, and long-lived sandboxes.

What problem it is solving:

- long-running agent compute,
- persistent sandboxes,
- workspace lifecycle management,
- infrastructure for many agent sessions.

Strengths:

- very complete runtime/platform story,
- strong persistence and volume model,
- built for operational scale,
- explicit file and Git tooling.

Weaknesses:

- heavier platform footprint,
- storage is a platform primitive, not obviously an agent-audit-first filesystem design,
- branching and review semantics are still less native than in an overlay-first system.

Threat to `agent-filesystem`:

- For organizations asking "how do I run thousands of agent sandboxes," Daytona is much more complete today.

Opportunity for `agent-filesystem`:

- Be the workspace substrate or collaboration layer for a Daytona-like runtime, rather than the runtime itself.

### Modal

What it is:

- Modal offers Sandboxes, Volumes, and filesystem snapshots.
- Its docs describe Volumes as a high-performance distributed file system.
- The docs are explicit that volumes use explicit commit/reload semantics and that concurrent modification of the same file should be avoided.

What problem it is solving:

- agent or app compute with durable shared storage,
- distributed persistent files for serverless workloads,
- snapshots for long-running sandboxes.

Strengths:

- strong managed infrastructure,
- clear primitives for compute and storage,
- snapshots and persisted volumes,
- very pragmatic for deployed systems.

Weaknesses:

- concurrency model is weaker than a transactionally rich workspace layer,
- explicit commit/reload creates a different developer model than normal POSIX,
- not especially agent-specific at the filesystem layer.

Threat to `agent-filesystem`:

- Strong platform option for teams that want storage plus compute with less custom infra.

Opportunity for `agent-filesystem`:

- Differentiate on agent-specific semantics, not generic distributed storage.

### Windmill AI Sandbox

What it is:

- Windmill combines `nsjail`-based sandboxing with persistent volumes.
- The docs explicitly describe the pattern as a sandboxed script plus a mounted volume for persistent agent state and artifacts.

What problem it is solving:

- safe workflow execution,
- persistent agent sessions inside orchestration pipelines,
- operational automation with agent state.

Strengths:

- very practical for workflow-native automation,
- easy way to persist agent memory and outputs,
- clean story for scheduled and orchestrated jobs.

Weaknesses:

- less of a full filesystem product,
- not trying to solve repo overlays or full agent workspace review semantics,
- best when your world already lives inside Windmill workflows.

### Arrakis

What it is:

- Arrakis is a self-hosted microVM sandbox for agent code execution and computer use.
- The repo emphasizes snapshot-and-restore, backtracking, VNC access, and secure MicroVM isolation.
- The README also says it uses overlayfs to protect the sandbox root filesystem.

What problem it is solving:

- strong isolation for untrusted code,
- backtracking during multi-step agent execution,
- computer use with GUI access,
- self-hosted control over runtime.

Strengths:

- strong safety model,
- snapshot and restore are first-class,
- directly aligned with agent experimentation and backtracking.

Weaknesses:

- heavier and more ops-intensive,
- more compute-runtime product than filesystem product,
- collaboration and durable queryable state are not the core differentiator.

### GitHub Agentic Workflows AWF

What it is:

- GitHub documents AWF as the default agent sandbox for GitHub Agentic Workflows.
- The docs describe the host filesystem as visible inside the container, with user paths like `$HOME`, `$GITHUB_WORKSPACE`, and `/tmp` writable and system paths read-only.

What problem it is solving:

- safe-ish agent execution in CI-like environments,
- keeping normal build tools available,
- controlling network egress and runtime behavior.

Strengths:

- strong compatibility,
- good for repo-centered automation,
- integrates well with existing GitHub workflows.

Weaknesses:

- not a distinct durable filesystem abstraction,
- persistence is basically the workflow/repo environment,
- not designed as a branchable workspace substrate.

### OpenHands

What it is:

- OpenHands' default runtime is Docker-based.
- Docs describe mounting a local filesystem into `/workspace`.
- The local runtime docs explicitly warn that it runs without sandbox isolation.

What problem it is solving:

- give coding agents a familiar working directory and shell,
- isolate actions when possible,
- keep the model close to normal developer workflows.

Strengths:

- extremely practical,
- very compatible with existing tools,
- low conceptual overhead.

Weaknesses:

- weak standalone storage story,
- auditability depends on outside tools,
- if you use local runtime, safety is poor.

This is the clearest reference point for the "just-bash" alternative.

### Kubernetes agent-sandbox

What it is:

- A Kubernetes SIG Apps project for isolated, stateful, singleton workloads.
- The repo frames it as ideal for AI agent runtimes.
- It introduces a `Sandbox` CRD with stable identity, persistent storage, lifecycle management, and optional warm pools/templates.

What problem it is solving:

- vendor-neutral lifecycle control for long-running stateful agent sandboxes on Kubernetes.

Strengths:

- strong control-plane model,
- good for platform teams,
- useful for persistent stateful agent sessions.

Weaknesses:

- very infrastructure-centric,
- not a developer-facing filesystem product,
- still needs a good workspace and audit model above it.

### Filesystem MCP Servers

Representative examples:

- `mark3labs/mcp-filesystem-server`
- similar filesystem MCP variants

What they are solving:

- safe structured file access for agents,
- strong root-path confinement,
- low-friction tool integration.

Strengths:

- simple,
- secure by design,
- easy to reason about.

Weaknesses:

- not a real filesystem for shells and compilers,
- weaker for build/test workflows,
- not naturally persistent or collaborative.

These are complements to `agent-filesystem`, not replacements.

## Where `agent-filesystem` Sits Today

From this repo today, `agent-filesystem` is more than one thing:

- a Redis-backed filesystem stored in standard Redis keys,
- an optional native module for `FS.*` commands,
- a FUSE mount,
- an NFS server,
- a Python client,
- an MCP server,
- a sandbox proof-of-concept,
- a search layer with BM25 over RediSearch.

The most important local facts:

- Storage uses standard Redis `HASH` and `SET` keys keyed by filesystem and absolute path.
- There is also an optional native Redis module with a flat path-to-inode design.
- File content is inline, not chunked.
- The repo already recognizes range I/O, inode IDs, chunking, and integrity tooling as future work.

That makes `agent-filesystem` unusually broad compared with most competitors. It already has more access modes than AgentFS, and a more explicit shared-storage story than many sandbox products.

## Strengths of `agent-filesystem`

### 1. Shared state is first-class

Unlike purely local overlays, `agent-filesystem` naturally supports:

- multiple clients,
- remote access,
- cross-language access,
- shared sessions through Redis,
- instant cleanup by key deletion.

That is a real advantage.

### 2. Inspectability is unusually good

Because the standard-key implementation stores data in readable Redis keys, humans and tools can inspect the filesystem without a proprietary binary format.

This is a meaningful operational strength.

### 3. Multiple interfaces already exist

`agent-filesystem` can be reached through:

- `FS.*`,
- FUSE,
- NFS,
- MCP,
- Python,
- CLI orchestration.

That is more surface area than most single-purpose competitors.

### 4. Text-oriented agent workflows are a good fit

The agent-friendly line editing, `GREP`, and BM25 search are better aligned with memory, docs, plans, logs, and code search than generic persistent volumes usually are.

### 5. Redis is a good fit for coordination and metadata

Redis gives:

- counters,
- TTLs,
- streams,
- pub/sub,
- low-latency metadata access,
- natural session state storage.

Those are underused strategic assets here.

## Weaknesses of `agent-filesystem`

### 1. It only partially solves the core agent workspace problem

`agent-filesystem` stores files well enough, but safe execution, branching, human review, and time-travel are not yet first-class product concepts.

That matters because those are exactly what the strongest competitors lead with.

### 2. The current data model is under pressure

The standard-key design is path-keyed, inline-content storage. The native module also uses a flat absolute-path mapping.

That creates known pressure around:

- subtree renames,
- chunking,
- range writes,
- large files,
- deep recursion and copy semantics,
- inode identity and harder POSIX features.

The backlog already points to:

- range-based I/O,
- inode-ID based namespace,
- integrity and repair,
- chunked payloads.

That is a strong signal that the current model is good but not yet durable as the long-term core.

### 3. No first-class overlay story

AgentFS's strongest idea is not "SQLite."
It is:

- keep the base tree clean,
- isolate agent writes,
- share or inspect a session,
- audit everything.

`agent-filesystem` does not yet have a comparable repo-overlay workflow.

### 4. Safety is adjacent, not intrinsic

The sandbox work in this repo is promising, but `agent-filesystem` itself is not yet the answer to "how do I safely run an agent against this workspace?"

### 5. Product messaging is still split

There is still some architectural tension in the repo between:

- "no custom module required, standard Redis structures,"
- "optional module for atomic operations,"
- earlier custom-type framing.

This is not fatal, but the project thesis is less crisp than the best competitors'.

## Competitive Scorecard

This table is intentionally directional, not a benchmark.

| Approach | Isolation | POSIX compatibility | Persistence | Branch / rollback | Shared sessions | Queryability / audit | Operational weight |
|---|---|---|---|---|---|---|---|
| Host repo + container + bash | Medium | High | Medium | Medium with Git | Medium | Low | Low |
| E2B-style sandbox | High | High | Medium | Medium | Low to Medium | Medium | Medium |
| Daytona / Modal / volume platforms | High | High | High | Medium to High | Medium | Medium | Medium to High |
| AgentFS overlay model | Medium to High | Medium to High | High | High | High | High | Medium |
| Filesystem MCP | High | Low | Medium | Medium | Low | Medium | Low |
| `agent-filesystem` today | Medium | Medium to High | High | Low to Medium | High | Medium to High | Medium |

The biggest gap for `agent-filesystem` is obvious:

- persistence is good,
- sharing is good,
- but branch/rollback/audit/session semantics lag behind the best agent-native designs.

## Strategic Options for `agent-filesystem`

### Option A: Double down on memory, docs, and text-state

Positioning:

- the best Redis-backed filesystem for agent memory, plans, logs, docs, and shared text artifacts.

What this means:

- embrace MCP, BM25, line editing, append-heavy workflows, TTLs, streams, and shared state,
- avoid pretending to be the best general-purpose build/test filesystem.

Pros:

- very defensible,
- aligned with current strengths,
- lower implementation risk.

Cons:

- concedes the coding-workspace category to AgentFS, E2B, Daytona, and plain local repos.

### Option B: Become a branchable agent workspace layer

Positioning:

- not just a filesystem in Redis,
- a sessioned overlay workspace for agents and humans.

What this means:

- base tree plus Redis overlay,
- whiteouts for deletes,
- shared session IDs,
- `snapshot`, `fork`, `diff`, `apply`,
- Redis Streams audit log,
- sandbox integration.

Pros:

- directly attacks the most interesting category,
- builds on Redis strengths,
- gives `agent-filesystem` a crisp product story.

Cons:

- materially more implementation work,
- requires data-model evolution and careful FUSE semantics.

### Option C: Use `agent-filesystem` as the control-plane/data-plane split

Positioning:

- real code execution happens in a shell/container/microVM,
- `agent-filesystem` stores the durable shared overlay, logs, search index, and session state.

What this means:

- do not replace bash,
- do not replace microVMs,
- become the persistent shared brain and workspace layer behind them.

Pros:

- strongest fit with the actual market,
- avoids fighting better-funded compute platforms on their own terms,
- lets `agent-filesystem` pair with "just-bash" rather than rejecting it.

Cons:

- slightly less elegant than a single all-in-one system,
- needs good integrations to feel cohesive.

## What I Would Build Next

### 1. Define the product thesis clearly

I would update the project framing to say one of these two things very clearly:

- `Redis Agent Filesystem` is a shared queryable workspace layer for agents, or
- `Redis Agent Filesystem` is a persistent Redis-backed memory and artifact filesystem for agents.

Right now it still reads as both a filesystem engine and a platform, without fully owning either story.

### 2. Add sessions, snapshots, and forks

This is the highest-leverage product gap.

Minimum feature set:

- `raf fork`
- `raf diff`
- `raf apply`
- `raf save`
- `raf rollback`
- `raf inspect`

Under the hood:

- parent pointer between sessions,
- read-through to base snapshot,
- copy-on-write upper layer in Redis,
- explicit whiteout records for deletes.

### 3. Add an audit log using Redis Streams

Record:

- file operations,
- shell commands,
- tool calls,
- timestamps,
- actor identity,
- session identity,
- before/after metadata where cheap.

This would give `agent-filesystem` something very few systems have natively: operational replay and workspace analytics.

### 4. Build a repo overlay mode

This is the direct answer to AgentFS.

Proposed model:

- read-only base: host repo, NFS export, or imported snapshot,
- writeable upper layer: Redis overlay,
- merged view: FUSE mount or sandbox mount,
- human tools: `diff`, `apply`, `export`, `gc`.

The killer user story becomes:

1. Mount repo as base.
2. Give each agent a Redis overlay session.
3. Let humans join live.
4. Inspect exact diffs.
5. Apply back to Git when ready.

### 5. Land the backlog items that protect scale

The backlog already points the right way:

- range-based I/O,
- inode IDs,
- chunked payloads,
- integrity checks,
- repair tools.

I would treat these as prerequisites for any serious coding-workspace push.

### 6. Add conflict and concurrency semantics

Today shared access is a strength, but it is not yet well-governed.

Needed concepts:

- session ownership,
- optimistic concurrency checks,
- last-writer-visible markers,
- append-only files,
- read-only branches,
- quotas per session.

### 7. Separate text-first and binary-heavy modes

There should probably be two storage profiles:

- text mode,
- large-file mode.

Text mode:

- inline content,
- line editing,
- search indexing.

Large-file mode:

- chunked blobs,
- ranged reads/writes,
- weaker inline editing assumptions.

### 8. Keep the shell in the product

The "just-bash" instinct is correct.

For code agents, the shell is not optional.
It is the center of gravity.

The question is not whether to replace bash.
The question is where bash points.

My recommendation:

- keep bash,
- keep normal tools,
- give them a safer and more inspectable filesystem substrate.

That is stronger than trying to replace the shell with only filesystem tools.

## Alternative Approaches That Might Beat `agent-filesystem` for Some Use Cases

### 1. Just bash plus a sandbox

Best when:

- the main job is coding, building, testing, and using normal tools,
- you do not need shared remote state,
- Git already provides most branching/review semantics.

Best implementation:

- per-task container or microVM,
- per-task Git worktree or branch,
- snapshots if needed,
- logs outside the workspace.

Why it can beat `agent-filesystem`:

- least impedance mismatch,
- fewer custom semantics,
- great developer ergonomics.

Why `agent-filesystem` can still matter:

- shared memory,
- audit trail,
- cross-session artifacts,
- search and coordination,
- remote inspection.

### 2. Git worktrees as the primary workspace layer

Best when:

- the canonical asset is a Git repo,
- human review is the dominant workflow,
- state should stay in normal files.

This is often the right default for coding agents.

`agent-filesystem` should consider integrating with this model instead of fighting it.

### 3. OverlayFS on local disk

Best when:

- single-host local execution is enough,
- you want AgentFS-like repo protection with minimal infra,
- you do not need distributed state.

This could be a simpler technical path than Redis overlays for a local-only edition.

### 4. MicroVM platform plus object or volume storage

Best when:

- security and execution fidelity dominate,
- you can afford heavier infrastructure,
- you want browser and GUI support,
- agents are long-running and compute-heavy.

This is where E2B, Daytona, Arrakis, and similar systems are strongest.

## Recommended Product Direction

If the goal is to make `agent-filesystem` a more thorough and well thought through solution, I would recommend this thesis:

`Redis Agent Filesystem` should become the shared, queryable, branchable workspace layer for agents, not merely a filesystem stored in Redis.

That implies:

- keep real shells,
- pair with containers or microVMs for execution,
- add overlay sessions and snapshots,
- make diffs and audit first-class,
- use Redis as the collaboration and event backbone,
- continue to shine in text-heavy memory, artifact, and search workflows.

In short:

- do not abandon bash,
- do not try to become just another hosted sandbox,
- do become the best agent workspace substrate behind those things.

## Concrete Near-Term Roadmap

### P0: Product clarity

- Tighten README and docs around the main thesis.
- Explain the relationship between standard-key storage, optional module commands, mounts, MCP, and sandboxing.

### P1: Session model

- Add session IDs, snapshots, forks, and diffs.
- Introduce Redis Streams audit logging.

### P2: Overlay coding workflow

- Base tree plus Redis overlay.
- Merged mount for agent shells.
- `apply` back to Git or host directory.

### P3: Data-model hardening

- Range I/O.
- Chunked storage.
- Inode-ID namespace.
- Integrity and repair tooling.

### P4: Runtime integrations

- First-class Docker/nsjail reference sandbox.
- Optional microVM integration path.
- Good Git worktree workflow.

## Sources

External:

- AgentFS docs: https://docs.turso.tech/agentfs/guides/sandbox
- AgentFS GitHub: https://github.com/tursodatabase/agentfs
- E2B docs overview: https://e2b.dev/docs
- E2B filesystem docs: https://e2b.dev/docs/filesystem
- E2B volumes docs: https://e2b.dev/docs/volumes
- E2B GitHub: https://github.com/e2b-dev/E2B
- Daytona architecture docs: https://www.daytona.io/docs/en/architecture/
- Daytona GitHub: https://github.com/daytonaio/daytona
- Modal sandboxes and volumes docs: https://modal.com/docs/guide/sandboxes
- Modal filesystem docs: https://modal.com/docs/guide/sandbox-files
- Modal volumes docs: https://modal.com/docs/guide/volumes
- GitHub Agentic Workflows sandbox docs: https://github.github.com/gh-aw/reference/sandbox/
- Windmill AI sandbox docs: https://www.windmill.dev/docs/core_concepts/ai_sandbox
- OpenHands runtime docs: https://docs.all-hands.dev/openhands/usage/runtimes/docker
- OpenHands runtime architecture docs: https://docs.all-hands.dev/openhands/usage/architecture/runtime
- OpenHands local runtime docs: https://docs.all-hands.dev/modules/usage/runtimes/local
- Arrakis GitHub: https://github.com/abshkbh/arrakis
- Kubernetes agent-sandbox GitHub: https://github.com/kubernetes-sigs/agent-sandbox
- Filesystem MCP server: https://github.com/mark3labs/mcp-filesystem-server
- MCP shell: https://github.com/sonirico/mcp-shell

Local repo references:

- `README.md`
- `BACKLOG.md`
- `module/fs.h`
- `redisclaw/README.md`
