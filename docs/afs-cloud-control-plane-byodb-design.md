# AFS Cloud Control Plane + Customer-Hosted Redis (BYODB) Design

## 1. Problem Statement

You want a hosted AFS control plane with two deployment modes:

1. **Built-in database (quick start):** AFS provisions and manages a Redis instance for the customer so they can create a workspace immediately.
2. **Bring your own database (BYODB):** Customer points AFS to their own Redis where workspace file/content data lives.

A hard requirement for BYODB is that the customer's Redis can remain **fully private** (on laptop, inside a VPC, or in an on-prem network) and **not reachable from the cloud control plane**.

That requirement implies a split architecture: cloud components cannot assume direct Redis access for core control-plane operations.

---

## 2. Core Architecture Decision

Adopt a **split-plane model**:

- **Control Plane (Cloud):** identity, organizations, workspaces, policy, orchestration, metadata index, audit, billing, API, UI.
- **Data Plane (Customer side):** Redis module data, mount process, optional local connector/agent.

The key design rule:

> The cloud control plane never directly depends on customer Redis reachability.

Instead, it communicates with a **customer-side connector** (or client-desktop runtime) over outbound-initiated secure channels, and stores only minimal cloud metadata needed for orchestration.

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

## 3.3 Built-in Database Mode

- A managed Redis (or Redis-compatible service) deployed per tenant/project by AFS.
- Same logical model and APIs as BYODB, but connector may be provider-managed or optional.
- This avoids feature divergence: “built-in” and BYODB share orchestration contracts.

---

## 4. Data Ownership and Boundaries

## 4.1 What lives in cloud metadata DB (must not require Redis access)

- Tenants/orgs/users/roles
- Workspace registry (workspace IDs, names, owner, region, status)
- Data-plane binding records (mode = built-in or BYODB, connector ID, last-seen)
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

## 5. Connectivity Model (No Inbound Requirement)

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
   - Built-in mode: provider-managed connector/service executes.
   - BYODB mode: customer connector executes.
4. Connector creates root structures in Redis and returns success/failure.
5. Cloud updates state to `READY` and emits audit event.

## 6.2 Mount on Desktop

1. Desktop resolves workspace via cloud metadata.
2. Desktop learns connection profile:
   - Built-in: managed endpoint + credentials brokered by cloud.
   - BYODB: local/org profile (may be local hostname, private DNS, unix socket tunnel, etc.).
3. Mount process interacts with Redis data plane directly when possible.
4. Cloud receives session telemetry, not file payload.

## 6.3 Checkpoint / Restore

- API writes intent in cloud.
- Connector performs operation in customer Redis.
- Connector emits immutable result record with checksum/size/time.
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

Avoid two product architectures by using one abstract interface:

- `DataPlaneProvider` interface
  - `InitWorkspace`
  - `DeleteWorkspace`
  - `CreateCheckpoint`
  - `RestoreCheckpoint`
  - `GetWorkspaceStats`
  - `Health`

Built-in database implementation is just another provider behind the same contract.

Benefits:

- Feature parity
- Same APIs/UX
- Easier testing and migration

---

## 10. Migration and Onboarding Flows

## 10.1 Day-0 Quick Start (Built-in)

- Customer signs up -> workspace ready in minutes.
- No infra setup.

## 10.2 Upgrade to BYODB

- Install connector in target environment.
- Register BYODB binding.
- Optional data migration job from built-in to BYODB.
- Switch workspace binding atomically.

## 10.3 BYODB Air-gapped-ish Setup

- Connector in customer network with egress-only HTTPS to cloud.
- Redis stays private; cloud never accesses it directly.

---

## 11. Suggested API Shape

- `POST /workspaces` (includes `dataPlaneBindingId`)
- `POST /dataplanes/bindings` (type: `BUILT_IN|BYODB`)
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
2. **Ship connector as first-class product** with auto-update, version skew policy, and strong observability.
3. **Define SLOs separately**:
   - Cloud API availability
   - Connector online rate
   - Workspace operation latency by mode
4. **Document trust boundaries clearly** for enterprise security reviews.
5. **Implement graceful degradation**:
   - If connector offline, cloud UI still shows metadata and last known state.

---

## 13. Risks and Mitigations

- **Risk: Connector offline blocks BYODB operations.**
  - Mitigation: queued intents, retries, clear UX status, optional HA connectors.

- **Risk: Metadata/content divergence.**
  - Mitigation: reconciliation jobs + periodic signed inventory snapshots.

- **Risk: Feature skew between built-in and BYODB.**
  - Mitigation: single provider contract + conformance tests.

- **Risk: Enterprise security objections to cloud-held secrets.**
  - Mitigation: local secret references + customer-managed KMS + no-secret mode.

---

## 14. Proposed Phased Delivery

### Phase 1: Metadata Split + Built-in DB
- Introduce cloud Postgres metadata schema.
- Keep built-in Redis path working end-to-end.
- Abstract operations behind `DataPlaneProvider`.

### Phase 2: BYODB Connector (Outbound)
- Connector enrollment + mTLS channel.
- Workspace init/checkpoint operations via job dispatch.
- Health and heartbeat surfaces in UI/API.

### Phase 3: Migration + Enterprise Hardening
- Built-in <-> BYODB migration tooling.
- Advanced policy controls, audit exports, private connectivity options.
- HA connector deployments and DR guidance.

---

## 15. Bottom Line

Yes—this can cleanly support both "works out of the box" and "customer-owned private Redis" if you treat Redis as **data-plane storage**, move control-plane truth to a dedicated cloud metadata store, and rely on an outbound customer connector for BYODB orchestration.

That gives:

- Fast onboarding via built-in managed Redis.
- Strict network/data-boundary compliance for enterprises.
- A single product model instead of two divergent architectures.
