# Tuma — Technical Design

Webhook reliability layer. Self-hosted, open source, built on Temporal for durable execution. Serves two purposes: (1) a standalone OSS product for teams who don't want to lose webhooks, and (2) the event-delivery backbone for koto7's headless logistics engine, running as its own dedicated self-hosted instance.

Throughout this document, **[OSS v1]** marks what ships in the first release. Everything else is **[Phase 2]** — real, designed, but not blocking.

---

## 1. Scope

Multi-tenancy, billing, and the admin console only matter if Tuma becomes a hosted product for *other* companies later. koto7's own production use doesn't need any of it — it just needs the core reliability primitives, running well, single-tenant. That reframing is what defines this cut:

| Area | OSS v1 | Phase 2 |
|---|---|---|
| Connections (create, view, configure) | ✅ | — |
| Retry policy (attempts, delay, backoff) | ✅ fully configurable, no gating | — |
| Issues / DLQ (view, bulk replay, per-event replay) | ✅ | Alert rules, assignment, resolution history |
| Data retention (configurable window) | ✅ | — |
| Metrics dashboard | Basic (throughput, error rate) | Latency percentiles, backpressure, 90-day history |
| Transformations | — | ✅ sandboxed JS transform editor |
| Users / seats | Single operator account | Multi-user, roles, invites |
| Multi-tenancy | — (single-tenant per deployment) | ✅ tenant isolation, per-tenant quotas |
| Admin console | — | ✅ cross-tenant ops, impersonation |
| Billing | — | ✅ Free/Pro tiers |

There is no Free/Pro split in v1. A self-hosted, single-tenant deployment doesn't need one — every feature above is just "the product."

---

## 2. Architecture overview

One Postgres instance does double duty: Temporal's persistence + visibility store, and Tuma's own application schema (connections, deliveries, issues, retention, sessions). No Redis, no Elasticsearch, no RocksDB in v1 — the actual latency budget (providers give 5–30 seconds; a single indexed Postgres insert takes low single-digit milliseconds) doesn't require them, and one dependency is simpler to self-host and reason about than three.

**Services (docker-compose, [OSS v1]):**
- `postgres` — single instance, backs both Temporal and Tuma
- `temporal` — self-hosted Temporal Server (OSS, not Cloud), auto-setup image, internal network only, never publicly exposed
- `tuma` — Go binary, runs in two modes: `serve api` (ingress + REST API, stateless, horizontally scalable) and `serve worker` (Temporal worker, also stateless/scalable); same binary, different flags, splittable into separate containers for real scale
- `tuma-web` — static React build, served by the api container or a CDN
- reverse proxy (Caddy, default) — automatic TLS for self-hosters with no existing ingress; documented as optional for those who have their own

**koto7 production deployment**: identical topology, scaled out — multiple `tuma api` and `tuma worker` replicas behind a load balancer, managed/HA Postgres instead of the docker-compose container. Same architecture, not a fork of it.

---

## 3. Core design principles

These are load-bearing and should not be relitigated per-PR:

1. **Durable-write-before-ack.** The event must be committed to Postgres *before* returning 200 to the source. Acking first and persisting async recreates the exact failure mode Tuma exists to prevent.
2. **At-least-once, never exactly-once.** Every outbound delivery carries a stable delivery ID header so destinations can dedupe. State this plainly in user-facing docs — promising more would be a lie that breaks trust the moment someone hits a retry.
3. **Temporal is not the system of record for payloads.** Workflow history handles orchestration state (retry timing, in-flight status). Retention, payload storage, and everything the UI queries and filters on lives in Tuma's own Postgres schema, written by activities as terminal events happen.
4. **Workflow versioning is mandatory, not optional.** Any change to a workflow function's control flow gets a `GetVersion()` check. This is a PR review checklist item, not a "remember to." (See §6.)
5. **Bulkheading per connection.** A cap on concurrent in-flight deliveries per connection, so one dead destination can't starve throughput for every other connection on the same deployment.
6. **Secrets are encrypted at rest.** Connection signing secrets are other companies' Stripe/GitHub credentials. App-level encryption, key from env var (self-host) or KMS (production).
7. **Temporal's own ports are never public.** Web UI and gRPC frontend have no built-in auth in OSS Temporal — full workflow visibility (i.e. every webhook payload) and control if exposed. Internal network only, enforced in the default docker-compose file, not left to documentation alone.

---

## 4. Data model (Postgres) — [OSS v1]

```sql
connections (
  id, name, source_type,        -- 'stripe' | 'github' | 'generic_hmac' | 'internal'
  inbound_path,                 -- e.g. /e/8f3a2c91
  destination_url, signing_secret_encrypted,
  retry_attempts, retry_first_delay_s, retry_backoff_factor,
  retention_days,
  created_at
)

events (
  id, connection_id, provider_event_id,   -- UNIQUE(connection_id, provider_event_id) — atomic dedup
  raw_headers, raw_payload,
  received_at, workflow_id
)

deliveries (
  id, event_id, attempt_number,
  status,                        -- 'pending' | 'delivered' | 'failed'
  response_code, latency_ms, attempted_at
)

issues (
  id, event_id, connection_id,
  reason, attempts_exhausted, first_failed_at,
  status,                        -- 'open' | 'resolved'
  resolved_at, resolved_by
)

sessions ( id, user_id, created_at, expires_at )   -- backs auth cookies
users ( id, email, password_hash, created_at )     -- single operator account in v1
```

Migrations via `golang-migrate` (or `goose`/`atlas` — pick one, doesn't matter which) from the very first schema, before there's any data to migrate against.

---

## 5. Ingestion (hot path) — [OSS v1]

```
1. HTTP POST arrives at /e/{inbound_path}
2. Look up connection by inbound_path (cached in-process, short TTL)
3. Adapter.Verify(headers, body, secret) — signature check, in-memory, no I/O
4. Adapter.ExtractEventID(headers, body)
5. INSERT INTO events (...) ON CONFLICT (connection_id, provider_event_id) DO NOTHING
     — atomic durable write + dedup in one statement
6. Return 200
7. Start Temporal workflow (async, after ack) keyed on event.id
```

Body size capped (reject oversized payloads before they hit Postgres). No separate dedup store needed — the Postgres unique constraint is the mechanism.

### Source adapters — [OSS v1: Stripe, GitHub, generic HMAC, internal/trusted-network]

```go
type SourceAdapter interface {
    Verify(headers http.Header, body []byte, secret string) bool
    ExtractEventID(headers http.Header, body []byte) string
}
```

A small registry (`stripe`, `github`, `generic_hmac` with configurable header name + algorithm, `internal` for trusted-network koto7-to-koto7 traffic with no signature required). This is the only provider-specific surface in the entire system — everything downstream is 100% shared code. New adapters (Shopify, a carrier API, etc.) are ~20–50 lines each and a natural place for community contributions post-launch.

---

## 6. Temporal workflow design — [OSS v1]

One workflow execution per event. Started after the event is durably persisted (§5), never before.

```
DeliveryWorkflow(eventID):
  loop attempt = 1..connection.retry_attempts:
    ExecuteActivity(DeliverActivity, eventID, attempt)   -- POST with X-Tuma-Delivery-Id header
    if success: RecordDelivered(activity); return
    if attempt < max: sleep(backoff(attempt))            -- Workflow.Sleep, durable
  RecordIssue(activity)   -- writes to `issues` table, exhausted retries
```

**Replay** = start a fresh workflow execution referencing the same event, or signal the original if still reachable — either way, goes through the same `DeliverActivity`, no separate code path.

**Versioning policy**: any change to the loop structure, backoff formula, or activity sequence gets wrapped in `workflow.GetVersion()`. In-flight workflows keep replaying against the branch they started on; new workflows take the new branch. Delete old branches once confident nothing old is still in-flight (delivery workflows are hours-scale, not months, so this window closes fast).

**Per-connection concurrency cap**: enforced via a bounded worker pool or semaphore keyed on connection ID — not raw Temporal task-queue concurrency alone — so one saturated destination doesn't consume the whole worker pool's capacity.

---

## 7. API — [OSS v1: REST + JSON]

```
POST   /api/connections
GET    /api/connections
GET    /api/connections/:id
PATCH  /api/connections/:id            -- retry policy, retention, destination
GET    /api/connections/:id/deliveries -- recent delivery log

GET    /api/issues?status=open|resolved
POST   /api/issues/:id/replay
POST   /api/issues/replay-bulk

POST   /api/auth/login   -- email/password, sets session cookie
POST   /api/auth/logout
```

Session-cookie auth, sessions table in Postgres (no Redis needed for this either). Single operator account in v1 — multi-user/roles is Phase 2.

---

## 8. Observability — [OSS v1]

- **Metrics**: Prometheus. Temporal's own metrics (task queue lag, worker pool, workflow completion) come for free from the Temporal Server. Tuma's own service exposes its own Prometheus counters/histograms (ingestion rate, delivery latency, DLQ depth, per-connection error rate) — same pipeline, no duplication through OTel.
- **Tracing**: OpenTelemetry, specifically for tracing (not metrics). Wire the Temporal Go SDK's OTel tracing interceptor on both client and worker so a single trace spans ingress → workflow → each retry activity → delivery, even across separate worker processes. This is what lets someone click into "why did this specific event take 40 seconds" instead of only seeing aggregate numbers.
- **Logs**: structured JSON, not free text.
- **Health**: `/healthz` and `/readyz` on the api service.
- **Dashboard**: ship a pre-built Grafana dashboard JSON in the repo alongside docker-compose, pointed at the bundled Prometheus scrape config. Low effort, high signal for self-hosters.

---

## 9. Frontend — [OSS v1: Connections + Issues screens]

React + TypeScript, Vite-bundled, no SSR (Next.js buys nothing for an authenticated internal dashboard). Component shape follows the existing prototype closely — Connections list → connection detail (URL, retry policy editor, recent deliveries) → Issues (filterable, bulk-select, replay, detail drawer). Metrics/Transformations/Users screens exist in the prototype and can be built later against the same API patterns; not part of the v1 critical path.

---

## 10. Deployment & self-host operations

- **Packaging**: docker-compose is the default and only [OSS v1] target. A Helm chart / k8s manifests for hardened production (which koto7 will eventually want) is a natural fast-follow, not a v1 blocker.
- **Scaling knobs**: `tuma api` and `tuma worker` are both stateless — add replicas behind a load balancer / more worker processes as needed, no code changes. Postgres is the one component that needs deliberate scaling (managed HA service) once real volume shows up.
- **Backups**: self-hoster's responsibility, but the docs ship concrete guidance (pg_dump / WAL archiving) rather than assuming it's obvious.
- **TLS/exposure**: Caddy reverse proxy included by default for zero-config public HTTPS; documented as swappable for anyone with existing ingress.

---

## 11. Explicitly deferred (Phase 2, not designed in detail here)

Multi-tenant isolation model, billing/Free-Pro tiers, admin console + impersonation, alert rules, transformations sandbox, multi-user roles. These exist in the UI prototype and remain the roadmap if/when Tuma becomes a hosted product for other companies — decoupled from koto7's own production timeline.
