# AFS Cloud Control Plane + Customer-Hosted Redis (BYODB) Design

## 1. Problem Statement

You want AFS to support three product experiences without forking the architecture:

1. **Self-hosted:** the current entirely local experience remains available.
2. **Cloud quick start:** AFS provisions and manages a Redis instance for the customer so they can create a workspace immediately.
3. **Cloud + customer database:** customer points AFS Cloud at their own Redis where workspace file/content data lives.

The cloud-attached customer database mode has two different rollout shapes:

- **Phase 1: External reachable database**
  - Customer attaches a Redis instance that the cloud control plane can reach.
  - This enables a fast first hosted release with full cloud UI behavior.
- **Phase 2: External hybrid database**
  - Customer keeps Redis fully private (laptop, VPC, or on-prem).
  - Cloud cannot assume direct Redis reachability and must rely on a customer-side connector.

The hard end-state requirement is that BYODB must support the **fully private** hybrid case without sacrificing the usefulness of the cloud UI for workspace lifecycle operations.

---

## 2. Core Architecture Decision

Adopt a **split-plane model** with one control plane and multiple data-plane binding types:

- **Control Plane (Cloud):** identity, organizations, workspaces, policy, orchestration, metadata index, audit, billing, API, UI.
- **Data Plane (Managed or Customer side):** Redis module data, mount process, and optionally a customer-side connector/agent.

The implementation rule is:

> The control plane depends on a `DataPlaneProvider` contract, not on raw Redis access everywhere in the codebase.

That allows AFS to ship in stages:

- `managed`: AFS-managed Redis for quick start.
- `external_reachable`: customer Redis, but directly reachable from cloud.
- `external_hybrid`: customer Redis remains private; cloud reaches it only through a connector.

For the final hybrid design rule:

> The cloud control plane never directly depends on Redis reachability for `external_hybrid` bindings.

---

## 3. High-Level Components

## 3.1 Cloud Control Plane

- **API Gateway / AuthN/AuthZ**
  - SSO/OIDC, API tokens, RBAC.
- **Workspace Service**
  - Workspace lifecycle: create, import, mount intents, checkpoint intents.
- **Metadata Service**
  - Canonical control-plane metadata DB (Postgres strongly recommended).
  - Stores workspace descriptors, connector bindings, health, policy tags, savepoint catalog metadata (not file blobs).
- **Orchestration Service**
  - Converts user intents to jobs/commands dispatched to connector(s).
- **Event Bus + Job Queue**
  - Async workflows and eventual consistency between cloud metadata and customer data plane.
- **Audit/Telemetry Service**
  - Signed audit logs, usage metering, diagnostics.

## 3.2 Customer Data Plane

- **Redis + AFS module**
  - Source of truth for filesystem/workspace contents in BYODB mode.
- **AFS Connector (new service)**
  - Runs in customer environment (desktop daemon, VM, or k8s deployment).
  - Maintains outbound mTLS WebSocket/gRPC stream to cloud.
  - Executes jobs from control plane: create workspace keyspace, materialize checkpoint, health probes, etc.
  - Publishes state/events back to cloud metadata service.
- **Desktop CLI/Mount runtime**
  - Can talk directly to customer Redis for local workspace operations.
  - Also talks to cloud APIs for identity, workspace discovery, and policy.

## 3.3 Data-Plane Binding Types

- **Managed**
  - A managed Redis (or Redis-compatible service) deployed per tenant/project by AFS.
  - Best for cloud quick start and lowest-friction onboarding.
- **External reachable**
  - Customer Redis is attached to AFS Cloud and reachable from the cloud control plane.
  - This is the recommended first hosted external-database release because it preserves full cloud UI behavior without requiring the connector on day one.
- **External hybrid**
  - Customer Redis is attached to AFS Cloud but remains private to the customer environment.
  - Control-plane operations execute through the customer connector over an outbound control channel.

All three binding types share one logical workspace model and one control-plane API surface.

## 3.4 Self-Hosted Mode

- Self-hosted remains a supported product mode.
- It can reuse core workspace/data structures, but it does not need to route through the hosted control plane.
- The hosted architecture should not regress or replace the current local-first experience.

---

## 4. Data Ownership and Boundaries

## 4.1 What lives in cloud metadata DB (must not require Redis access)

- Tenants/orgs/users/roles
- Workspace registry (workspace IDs, names, owner, region, status)
- Data-plane binding records (mode = `managed|external_reachable|external_hybrid`, connector ID, last-seen)
- Policy objects (retention policy, backup schedule intent, allowed operations)
- Checkpoint catalog metadata (checkpoint ID, timestamps, labels, size estimate, storage class)
- Event logs and audit records
- Billing/usage aggregates

## 4.2 What lives in customer Redis (BYODB)

- Workspace manifests/blobs/savepoints (actual user data)
- AFS module keyspace objects
- Any sensitive payload content the customer does not want in cloud

## 4.3 Optional mirrored index data (derived)

For UX speed, cloud may keep a derived, stale-tolerant index (e.g., recent workspace stats). It must be explicitly marked as advisory and reconstructed from connector events.

---

## 5. Connectivity Models

## 5.1 External Reachable Database

For the first hosted external-database release:

1. Customer registers an external Redis endpoint with AFS Cloud.
2. Control plane validates connectivity and capabilities directly.
3. Cloud executes workspace lifecycle operations directly against Redis via the provider contract.
4. AFS clients receive session bundles that let them talk to the Redis data plane directly.

This mode is simpler to ship first, but it is not sufficient for customers who require a private/on-prem data plane.

## 5.2 External Hybrid Database (No Inbound Requirement)

For strict private-network BYODB, use an **agent-initiated outbound control channel**:

1. Connector authenticates with short-lived credential bootstrap (device code, token exchange, or one-time enrollment secret).
2. Connector opens outbound mTLS stream to cloud broker.
3. Control plane sends signed job envelopes over this stream.
4. Connector executes against local Redis and returns signed results/events.
5. If disconnected, jobs queue with TTL and replay semantics.

This is the same pattern used by many hybrid-cloud systems and avoids requiring firewall openings or VPN from cloud to customer network.

---

## 6. Unified Workspace Lifecycle

## 6.1 Create Workspace

1. User calls cloud API `CreateWorkspace`.
2. Cloud writes workspace metadata with `state = PROVISIONING`.
3. Orchestrator dispatches `WorkspaceInitJob` to target binding:
   - `managed` / `external_reachable`: provider executes directly.
   - `external_hybrid`: customer connector executes.
4. Provider creates root structures in Redis and returns success/failure.
5. Cloud updates state to `READY` and emits audit event.

## 6.2 Mount on Desktop

1. Desktop resolves workspace via cloud metadata.
2. Desktop learns connection profile:
   - `managed`: managed endpoint + credentials brokered by cloud.
   - `external_reachable` / `external_hybrid`: local/org profile (may be local hostname, private DNS, unix socket tunnel, etc.).
3. Mount process interacts with Redis data plane directly when possible.
4. Cloud receives session telemetry, not file payload.

## 6.3 Checkpoint / Restore

- API writes intent in cloud.
- Provider executes operation:
  - `managed` / `external_reachable`: directly
  - `external_hybrid`: through connector
- Connector emits immutable result record with checksum/size/time when hybrid mode is in use.
- Cloud updates checkpoint catalog metadata.

---

## 7. Security Architecture

## 7.1 Identity and Trust

- Each connector has unique identity (x509 cert SPIFFE-style ID or equivalent).
- Tenant-scoped trust domain.
- Short-lived credentials issued by cloud CA/STS.

## 7.2 Authorization

- Cloud enforces user RBAC at API layer.
- Connector enforces job authorization by validating job signature + tenant/workspace scope.
- Optional policy-as-code guardrails in connector (deny destructive ops unless approved).

## 7.3 Secrets

- BYODB Redis credentials never need to be stored in cloud if customer uses local secret refs.
- If cloud escrow is needed, store encrypted with tenant KMS key and explicit opt-in.

## 7.4 Network

- Default: outbound 443 only from customer connector.
- No inbound firewall rules required.
- Optional PrivateLink/VPN for enterprises that want dedicated links.

---

## 8. Reliability and Consistency

- **Control plane metadata is authoritative for intent/state machine**.
- **Redis data plane is authoritative for actual file content**.
- Use explicit state transitions and idempotent jobs:
  - `PENDING -> RUNNING -> SUCCEEDED|FAILED|TIMED_OUT`
- All jobs carry idempotency key + expected version.
- Periodic reconciliation loop:
  - Connector sends heartbeat + capability/version + workspace digests.
  - Cloud reconciles drift and raises alerts.

---

## 9. Built-in DB vs BYODB: Keep One Contract

Avoid three divergent product architectures by using one abstract interface:

- `DataPlaneProvider` interface
  - `ValidateBinding`
  - `InitWorkspace`
  - `DeleteWorkspace`
  - `CreateCheckpoint`
  - `RestoreCheckpoint`
  - `IssueClientSession`
  - `ReadWorkspaceTree`
  - `ReadFileContent`
  - `GetWorkspaceStats`
  - `Health`

`managed`, `external_reachable`, and `external_hybrid` are implementations behind the same contract.

Benefits:

- Feature parity
- Same APIs/UX
- Easier testing and migration

---

## 10. Migration and Onboarding Flows

## 10.1 Day-0 Quick Start (Built-in)

- Customer signs up -> workspace ready in minutes.
- No infra setup.

## 10.2 Attach External Reachable Database

- Customer registers a Redis instance reachable from AFS Cloud.
- Cloud validates the database and enables full workspace lifecycle operations immediately.
- Optional data migration job from built-in to external reachable.

## 10.3 Upgrade to External Hybrid

- Install connector in target environment.
- Register hybrid binding and enroll connector.
- Optional data migration job from built-in or external reachable to hybrid.
- Switch workspace binding atomically.

## 10.4 BYODB Air-gapped-ish Setup

- Connector in customer network with egress-only HTTPS to cloud.
- Redis stays private; cloud never accesses it directly.

---

## 11. Suggested API Shape

- `POST /workspaces` (includes `dataPlaneBindingId`)
- `POST /dataplanes/bindings` (type: `MANAGED|EXTERNAL_REACHABLE|EXTERNAL_HYBRID`)
- `POST /dataplanes/bindings/{id}:validate`
- `POST /connectors/enroll`
- `POST /workspaces/{id}/checkpoints`
- `POST /workspaces/{id}/restore`
- `GET /workspaces/{id}/status`
- `GET /connectors/{id}/health`

Job/event topics (logical):

- `workspace.init.requested`
- `workspace.init.completed`
- `checkpoint.create.requested`
- `checkpoint.create.completed`
- `connector.heartbeat`
- `connector.alert`

---

## 12. Operational Recommendations

1. **Use Postgres for cloud metadata** (not Redis) to decouple control-plane durability and query patterns from customer data store topology.
2. **Ship `external_reachable` first, but do it behind the provider contract** so the hybrid connector can slot in later without rewriting the product.
3. **Ship connector as first-class product** with auto-update, version skew policy, and strong observability.
4. **Define SLOs separately**:
   - Cloud API availability
   - Connector online rate
   - Workspace operation latency by mode
5. **Document trust boundaries clearly** for enterprise security reviews.
6. **Implement graceful degradation**:
   - If connector offline, cloud UI still shows metadata and last known state.

---

## 13. Risks and Mitigations

- **Risk: Connector offline blocks BYODB operations.**
  - Mitigation: queued intents, retries, clear UX status, optional HA connectors.

- **Risk: Phase-1 external database users assume private/on-prem support immediately.**
  - Mitigation: label the first release clearly as `external reachable` until hybrid connector support exists.

- **Risk: Metadata/content divergence.**
  - Mitigation: reconciliation jobs + periodic signed inventory snapshots.

- **Risk: Feature skew between built-in and BYODB.**
  - Mitigation: single provider contract + conformance tests.

- **Risk: Enterprise security objections to cloud-held secrets.**
  - Mitigation: local secret references + customer-managed KMS + no-secret mode.

---

## 14. Proposed Phased Delivery

### Phase 1: Metadata Split + Managed DB
- Introduce cloud Postgres metadata schema.
- Keep built-in Redis path working end-to-end.
- Abstract operations behind `DataPlaneProvider`.

### Phase 2: External Reachable Database
- Add external database bindings that cloud can validate and reach directly.
- Keep the cloud UI fully functional for create/delete/checkpoint/browser flows.
- Reuse the same provider contract that hybrid mode will later use.

### Phase 3: BYODB Connector (Outbound)
- Connector enrollment + mTLS channel.
- Workspace init/checkpoint operations via job dispatch.
- Health and heartbeat surfaces in UI/API.

### Phase 4: Migration + Enterprise Hardening
- Built-in <-> external reachable <-> hybrid migration tooling.
- Advanced policy controls, audit exports, private connectivity options.
- HA connector deployments and DR guidance.

---

## 15. Bottom Line

Yes—this can cleanly support quick-start cloud, reachable external databases, fully private hybrid BYODB, and the existing self-hosted mode if you treat Redis as **data-plane storage**, move control-plane truth to a dedicated cloud metadata store, and keep every binding behind one provider contract.

That gives:

- Fast onboarding via built-in managed Redis.
- A practical first hosted external-database release before the connector exists.
- Strict network/data-boundary compliance for enterprises.
- A single product model instead of two divergent architectures.
