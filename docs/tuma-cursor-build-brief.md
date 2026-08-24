# Tuma OSS v1 — Build brief for Cursor

Companion to `tuma-technical-design.md`. That document is the source of truth for *why*; this one is the sequenced *what to build*. Feed both into Cursor — the design doc first, this brief second.

## Non-goals (say this explicitly, first, before anything else)

Do not build: multi-tenancy, billing, an admin console, transformations, multi-user roles/invites, Free/Pro gating of any kind. Every feature in this brief is unconditionally available — there is no paywall logic in v1. If a prompt or a piece of generated code introduces a "plan" or "tier" check, that's scope creep — flag it and remove it.

## Repo structure

```
/cmd/tuma              -- single binary, `tuma serve api` / `tuma serve worker`
/internal/api           -- REST handlers
/internal/workflow       -- Temporal workflow + activity definitions
/internal/adapters       -- SourceAdapter implementations (stripe, github, generic_hmac, internal)
/internal/storage        -- Postgres access layer, migrations
/migrations              -- golang-migrate SQL files
/web                      -- React + TS + Vite frontend
/deploy/docker-compose.yml
/deploy/grafana-dashboard.json
```

## Build order

1. **Schema + migrations.** Tables from design doc §4 (`connections`, `events`, `deliveries`, `issues`, `sessions`, `users`). Set up `golang-migrate` before writing any application code against the schema.
2. **Local Temporal dev environment.** Self-hosted Temporal via docker-compose, Postgres-backed persistence and visibility (no Elasticsearch). Confirm the Temporal Web UI and gRPC port are bound to the internal docker network only — verify this explicitly, don't assume the default compose template got it right.
3. **Ingestion endpoint + adapters.** Build the hot path exactly as sequenced in design doc §5: verify → `INSERT ... ON CONFLICT DO NOTHING` → ack → enqueue workflow. Implement `SourceAdapter` interface first, then `stripe`, `github`, `generic_hmac`, `internal` adapters against it. Write a test that asserts ack happens only after the Postgres write commits, not before.
4. **Delivery workflow + activities.** Implement per design doc §6. From the very first version, wrap any workflow control-flow decision in `workflow.GetVersion()` — even in v1 with no prior versions to migrate from, establish the pattern now so it's a habit before it's a requirement. Include the per-connection concurrency cap (semaphore or bounded pool keyed on connection ID) — build this before load-testing reveals you need it, not after.
5. **REST API.** Endpoints from design doc §7. Session-cookie auth backed by the `sessions` table — no Redis.
6. **Frontend — Connections + Issues only.** React + TS + Vite. Match the existing prototype's component shape for these two screens specifically; don't build Metrics/Transformations/Users screens yet even though they exist in the prototype.
7. **Observability.** Prometheus metrics on the Tuma service (ingestion rate, delivery latency, DLQ depth, per-connection error rate). Wire Temporal's OTel tracing interceptor on both client and worker. `/healthz` + `/readyz`. Structured JSON logging throughout — not an afterthought pass at the end.
8. **Docker-compose packaging.** Full stack: postgres, temporal (auto-setup, internal-network-only), tuma api, tuma worker, tuma-web, Caddy reverse proxy (default, documented as optional). Bundle the Grafana dashboard JSON and a Prometheus scrape config alongside it.
9. **Docs.** Quickstart (`docker compose up` → paste URL into Stripe → see it work), backup/restore guidance for the self-hoster's Postgres instance, and an explicit "at-least-once delivery, your handler must be idempotent" callout — this is a correctness contract with users, not optional documentation polish.

## Rules to hold constant across the whole build

- Every outbound delivery POST carries a stable `X-Tuma-Delivery-Id` header (design doc §3.2) — check this on the very first `DeliverActivity` implementation, not retrofitted later.
- Connection signing secrets are encrypted at rest from the first migration that creates the `connections` table — never land a version where they're stored plaintext, even temporarily during development, since that habit tends to survive into production.
- No feature reads from or writes to Temporal's visibility store for product data. If a Cursor-generated query is fetching connection/issue/event data via the Temporal client instead of Postgres, that's a violation of design doc §3.3 — redirect it to the Postgres layer.

## Definition of done for v1

A user can: `docker compose up`, create a connection through the wizard, paste the generated URL into Stripe (or GitHub), see a real event flow through to their destination, kill their destination app, watch retries happen and land in Issues, bring the destination back up, and replay the failed event successfully — all without touching a database console, and all visible in the bundled Grafana dashboard.
