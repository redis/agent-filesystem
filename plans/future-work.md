# Future Work

Last reviewed: 2026-05-02.

This is the only place for planned work. If a feature is implemented, move the
truth into the relevant current doc and remove it from this file.

## Cloud

- Add full OAuth/PKCE profile and refresh-token handling for the CLI.
- Store cloud tokens/profiles in OS-backed secure storage.
- Add a production secret-store boundary for provider and external database
  credentials.
- Add explicit external Redis validation and credential rotation.
- Implement AFS-managed Redis provisioning.
- Implement private BYODB connector support for customer Redis instances that
  are not reachable from AFS Cloud.
- Add cloud-connected FUSE/NFS mount mode if demand justifies it.

## Redis Array Storage

AFS should use Redis Array as the native live-root file-content backend when the
connected Redis database supports the Array command family.

Open implementation slices:

- capability detection and rollout guardrails
- Array-backed content reads/writes/truncation
- mixed `ext` and Array-backed workspace support
- sync integration for dirty ranges
- `ARGREP` integration where it preserves CLI grep semantics
- benchmarks for wire traffic, Redis memory, server work, local hashing, and
  grep latency

Current Redis databases without Array support must keep the existing external
string-key backend unchanged.

## Versioned Filesystem

- Add session-boundary auto-checkpoints.
- Add fork review and accept/reject flow.
- Add CLI `afs history` and `afs path history` commands.
- Add an indexed path-history endpoint only if current event filtering becomes
  too slow.
