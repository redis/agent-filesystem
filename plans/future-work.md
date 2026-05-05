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

## Redis Array Benchmarks

Redis Array is now an implemented content backend. The remaining follow-up work
is measurement, not basic enablement.

Open implementation slices:

- compare `ext` versus `array` wire bytes and command counts under sync churn
- compare Redis memory and server work per logical MiB
- measure `ARGREP` prefilter impact on grep latency and false-positive rates
- capture durable benchmark summaries in `docs/internals/performance.md`

## Versioned Filesystem

- Add session-boundary auto-checkpoints.
- Add fork review and accept/reject flow.
- Add CLI `afs history` and `afs path history` commands.
- Add an indexed path-history endpoint only if current event filtering becomes
  too slow.
