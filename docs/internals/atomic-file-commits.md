# Atomic file commits

AFS clients coordinate file publication directly in Redis. Different control
planes do not isolate clients that address the same workspace storage ID, and
they do not merge simultaneous edits.

## Read, stage, compare, publish

Sync reads an uncached file snapshot with matching inode/revision metadata
before and after the bytes. It also checks the parent identity. Uploads carry
that observation into `WithExpectedStat` and `WithExpectedParent`; absence is an
explicit conditional-create precondition. Recovery scans, downloads, and save
use these observations instead of pairing bytes with an unrelated later stat.
The sync state stores the remote inode and revision separately from its local
state-write version, so same-size edits within one millisecond remain visible.

File writers build a private staged String or Array. Delta uploads copy existing
content within the selected Redis database, then patch the private copy. New
chunked files require all chunks. The existing parent-chain WATCH transaction
protects path bindings, and the publication Lua script checks the expected
revision, workspace generation, and native-session lease where applicable.

The short publication script consumes the stage and updates live content,
metadata (including mode), revision, accounting, dirty markers, and the change
journal without another command interleaving. A stale condition returns a
conflict before live data changes. Sync preserves its local candidate for the
existing conflict-copy/recovery flow; it never retries a stale candidate against
a newly accepted baseline. Native range operations retain their existing
operation-specific retry behavior.

`MutationResult` returns this operation's exact committed stat. A later peer
write therefore cannot be recorded as this upload's baseline. History likewise
uses the frozen published bytes, not a post-commit read of the live file.

## Temporary keys and acknowledgment

All keys retain the existing workspace hash tag `afs:{<storage-id>}:`.

| Key suffix | Purpose | Lifetime |
| --- | --- | --- |
| `content:<inode>:stage:<nonce>` | Unpublished complete content | One hour, normally unlinked on completion |
| `retired:<revision>` | Replaced live content, removed asynchronously | One hour, normally unlinked immediately |
| `commit:<revision>` | SHA-256 digest of the publication request | Seven days |

Publication renames old content out of the way, then renames the staged value
into the live key. The live key loses the staging expiry. Empty files consume a
stage but need no live content key. UNLINK avoids synchronously freeing large
retired values in the commit script.

The operation ID is allocated once per publication. A matching receipt makes a
transport retry acknowledge the original success even if a peer subsequently
replaced or deleted the file. It does not replay bytes or append another journal
event. Receipts are bounded by TTL but their memory use scales with write rate.
Pub/Sub provides supplemental wakeups; the mutation itself writes the durable
journal once, including its operation ID for content publications.

This does not persist pending operation IDs in the local sync state. After a
process restart, sync reconciles observed content/revisions rather than resuming
the same operation. If a failed request's receipt cannot be read, the caller
receives an error with an uncertain outcome. Redis persistence/failover settings
still determine durability. Lua prevents interleaving but does not roll back
runtime errors: expected key types and numeric bookkeeping are checked before
mutation, without claiming recovery from every server/storage failure.

## Deployment and scope

Upgrade every client writing the shared tree and restart its sync or mount
session. An older client can bypass the new sync precondition; raw Redis writers
can bypass the protocol entirely. Use the existing workspace-generation fencing
for restore/migration; no new storage schema or control-plane lock is introduced.
Legacy sync state remains readable, and content is verified before adopting a
new revision baseline.

The guarantee is per file publication, not a multi-file transaction or automatic
text merge. Save/checkpoint semantics remain as documented elsewhere. No running
control plane needs to mediate individual file I/O.

## Validation

Real Redis tests cover competing creates and overwrites, inline and chunked
uploads, renamed parents, stale cache reads, exact result/history attribution,
lost acknowledgments after newer writes/deletes, malformed bookkeeping, journal
deduplication, temporary-key cleanup, and selected-database COPY behavior.
String storage is exercised with Redis 7.4.2 and Array storage with Redis 8.10.0.
Concurrent cases also run repeatedly with Go's race detector. Performance
measurements are recorded in [performance.md](performance.md).
