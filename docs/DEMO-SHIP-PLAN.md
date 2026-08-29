# Tuma — Demo ship plan

**Status:** Active plan  
**Last updated:** 2026-08-28  
**Sequence:** EasyPost adapter → Playground → cold outreach

This document is the single source of truth for getting Tuma demo-ready. Do not ship the playground before the EasyPost adapter is done and verified.

---

## Overview

| Phase | Deliverable | Blocks |
|---|---|---|
| **1** | EasyPost adapter | Playground provider dropdown, logistics cold outreach |
| **2** | Playground (`/playground`) | Public hands-on demo, HN, cold email links |
| **3** | Marketing + outreach | Link from tuma.koto7.io, email copy |

**Already done (infra):** demo droplet, CI/CD, deploy key, DNS (`tuma-demo.koto7.dev`).

**Explicitly deferred:** Terraform/Helm, K8s chart, hosted SaaS, multi-tenancy. See `docs/PRODUCTION.md` (to write post-demo) for self-host scale paths.

---

# Phase 1 — EasyPost adapter

Fourth `SourceAdapter` alongside `stripe`, `github`, `generic_hmac`, `internal`. **No infra or architecture changes** — new file + registry entry + migration + tests + UI label.

## Interface (unchanged)

```go
type SourceAdapter interface {
    Verify(headers http.Header, body []byte, secret string) bool
    ExtractEventID(headers http.Header, body []byte) string
}
```

Location: `internal/adapters/easypost.go`, registered in `internal/adapters/adapters.go`.

---

## 1. EasyPost webhook contract

### Signature schemes

| Scheme | Header | Notes |
|---|---|---|
| **Legacy** | `X-Hmac-Signature` | HMAC-SHA256 over raw body. No replay protection. **Do not build v1 around this.** |
### Signature schemes

| Scheme | Header | Status |
|---|---|---|
| **Legacy (ship now)** | `X-Hmac-Signature` | Ported from official Python/Node SDKs |
| **v2 (future)** | `x-hmac-signature-v2` | Not in SDKs yet — add when EasyPost ships client support |

**Critical implementation rule:** Port legacy from SDK source. Do **not** implement v2 from docs prose alone.

**Acceptance test:** real Test-mode webhook → confirm header is `X-Hmac-Signature` (expected). If v2 headers appear, extend adapter before outreach.

### Operational constraints

| Constraint | Value | Implication |
|---|---|---|
| **Response window** | **7 seconds** | Tightest adapter so far (Stripe ~20–30s, GitHub ~10s). Hot path is already ms-level Postgres insert + ack — no architecture change, but **assert in test** that ingest completes well under 7s. |
| **Source retries** | Up to 6 retries on non-2xx | Source-side; Tuma's durable-write-before-ack already handles this. |
| **Dedup** | Top-level JSON `id` on Event object | Same `ON CONFLICT (connection_id, provider_event_id) DO NOTHING` path as Stripe/GitHub. |

### Out of scope (v1)

- Basic-auth webhooks (EasyPost supports it; HMAC-only for v1)
- Legacy `X-Hmac-Signature` support (add only if a real customer needs it)

---

## 2. What does not need building

- No new ingestion logic (verify → extract ID → insert → ack → workflow)
- No new retry/Issues logic
- No new infra services

---

## 3. Implementation checklist

### Backend

- [ ] **1.** Add `internal/adapters/easypost.go` — `Verify`, `ExtractEventID`
- [ ] **2.** Port v2 validation from official EasyPost SDK source (timestamp check **first**, then HMAC — same order as EasyPost)
- [ ] **3.** Use `hmac.Equal` for timing-safe comparison (same as other adapters)
- [ ] **4.** Register `"easypost"` in adapter registry
- [ ] **5.** Migration: extend `connections.source_type` CHECK to include `'easypost'`
- [ ] **6.** Unit tests with vectors copied from SDK test fixtures (if available) + table-driven cases
- [ ] **7.** Integration test: real EasyPost Test-mode webhook against local `/e/{path}`

### Frontend / UX

- [ ] **8.** Add EasyPost to Connections wizard source list (`ConnectionsPage.tsx` — name, sub-label e.g. "Tracking & shipping events")
- [ ] **9.** Placeholder hint for signing secret field (from EasyPost webhook settings)

### Scripts & docs

- [ ] **10.** `scripts/simulate-easypost-event.py` — signed inbound events for local/playground use (port v2 signing from same SDK reference as Go adapter)
- [ ] **11.** README: add EasyPost to adapter list + one-line setup note (webhook URL + secret from EasyPost dashboard)
- [ ] **12.** Team test plan: optional Test 1b — EasyPost happy path (after Stripe Test 1)

### Verification gate (Phase 1 done when)

- [ ] Real Test-mode EasyPost webhook → 200 → delivery to destination
- [ ] Invalid signature → 401
- [ ] Duplicate event `id` → 200 both times, one delivery
- [ ] Ingest p99 < 2s on demo hardware (comfortable margin under 7s)

**Estimated effort:** 2–3 dev days (signature port + Test-mode account setup is the long pole).

---

# Phase 2 — Playground

Public `/playground` route in the same React app (no iframe). Three panes: **live status**, **provider simulator**, **destination log**. Log windows only — not a real terminal.

**Header copy (fixed):**  
`Simulated Stripe · Demo destination · Real Tuma stack`

Depends on Phase 1 for provider dropdown (Stripe, GitHub, EasyPost, Internal).

---

## Design principles

1. **Per-visitor isolation** — cookie-keyed session owns one ephemeral connection + one virtual destination. **Not a shared singleton.**
2. **Same origin** — `/playground` in `tuma-web`; no iframe, no third-party cookie issues.
3. **Honest labeling** — simulators and demo destination clearly labeled; real ingest/delivery/workflow in the middle.
4. **Demo-only code paths** — `TUMA_DEMO_MODE=true` on demo droplet only; self-hosters never see playground routes.
5. **Rate limiting** — all unauthenticated playground writes get per-IP token buckets.

---

## Architecture

```text
Browser (/playground)
  │
  ├─ Cookie: tuma_playground=<session_uuid>
  │
  ├─ GET  /api/playground/bootstrap     → create/refresh session + connection
  ├─ GET  /api/playground/status        → read-only live state (pane 1)
  ├─ GET  /api/playground/stream        → SSE: sim log + destination log
  ├─ POST /api/playground/simulate      → signed fake provider event(s)
  ├─ POST /api/playground/break         → destination returns 502
  ├─ POST /api/playground/fix           → destination OK again
  └─ POST /api/playground/replay        → replay latest open issue for this session

Tuma (normal path)
  simulate → POST /e/{inbound_path} → verify → Postgres → Temporal → deliver
                                                          ↓
                              POST /playground/r/{session_id}/hook  (virtual receiver)
```

Each visitor's connection `destination_url` points at **their** virtual receiver. Visitor A clicking Break does not affect Visitor B.

---

## Per-visitor sessions (required — not optional)

### Problem with shared singleton

One seeded connection + nightly reset does **not** fix concurrent visitors. Cold email / HN → multiple prospects at once → one person Breaks destination → next visitor sees Failing for no reason. **Ship-blocking.**

### Cookie

| Property | Value |
|---|---|
| Name | `tuma_playground` |
| Value | Session UUID |
| HttpOnly | true |
| SameSite | Lax |
| Secure | true when HTTPS |
| Max-Age | 2h sliding (refresh on activity) |

### Schema migration

```sql
CREATE TABLE playground_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    connection_id    UUID NOT NULL UNIQUE REFERENCES connections(id) ON DELETE CASCADE,
    fail_destination BOOLEAN NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_playground_sessions_last_seen ON playground_sessions(last_seen_at);

ALTER TABLE connections ADD COLUMN is_playground BOOLEAN NOT NULL DEFAULT FALSE;
```

Extend `source_type` CHECK if needed (already includes easypost from Phase 1).

### Bootstrap (`GET /api/playground/bootstrap`)

1. Valid cookie → refresh `last_seen_at`, return session summary.
2. Missing/expired → create:
   - `playground_sessions` row
   - Connection: `name="Playground"`, `source_type="stripe"` (sim can override provider), `is_playground=true`
   - `destination_url`: `{INTERNAL_BASE}/playground/r/{session_id}/hook`
   - **Short demo retries:** `retry_attempts=3`, `retry_first_delay_s=5`, `retry_backoff_factor=2` (~15s to Issues, not 8 minutes)
   - Set cookie, return payload

### Garbage collection

Background goroutine (demo mode only), every 15 min:

- Delete sessions where `last_seen_at < NOW() - 2 hours` (CASCADE → connection → events → issues)
- Cap: 500 active sessions; evict oldest idle if over cap

### Console hygiene

- `GET /api/connections` excludes `is_playground=true`
- Playground rows never appear in authenticated console

---

## Virtual destination receiver

On **tuma-api** (demo mode only) — no extra container for v1.

### Route

`POST /playground/r/{session_id}/hook`

1. Load session; 404 if unknown.
2. `fail_destination=true` → **502** (retries → Issues).
3. Else → **200**, append to ring buffer (last 50), push to SSE.

### SSE (`GET /api/playground/stream`)

```text
event: sim
data: {"ts":"...","line":"→ POST /e/abc 200 {\"received\":true}"}

event: dest
data: {"ts":"...","line":"POST /hook 200 42ms","delivery_id":"...","body_preview":"..."}
```

In-memory subscribers per session. Acceptable for demo; logs clear on API restart (session remains valid).

---

## Simulator (`POST /api/playground/simulate`)

Unauthenticated; session from cookie only.

```json
{
  "provider": "stripe" | "github" | "easypost" | "internal",
  "count": 1,
  "duplicate_event_id": "optional"
}
```

1. Resolve session → connection → signing secret.
2. Build signed payload via `internal/playground/simulate.go` (reuse adapter signing logic / port from Python scripts).
3. Internal HTTP POST to `/e/{inbound_path}`.
4. Append to sim log → SSE.

---

## Failure story controls

| Action | Endpoint | Effect |
|---|---|---|
| Break destination | `POST /api/playground/break` | `fail_destination=true` |
| Fix destination | `POST /api/playground/fix` | `fail_destination=false` |
| Replay latest issue | `POST /api/playground/replay` | Newest open issue for session's connection |

### Pane 1 — live status (`GET /api/playground/status`, poll 3s)

- Status pill (Delivering / Degraded / Failing)
- Delivered count (24h)
- Open issues count
- Last 3 deliveries
- Inbound URL (read-only, transparency)

---

## Rate limiting (required before ship)

In-memory token bucket per client IP (`X-Forwarded-For` from Caddy).

| Endpoint | Limit |
|---|---|
| `POST /api/playground/simulate` | 20/hour/IP, burst 5 |
| `POST /api/playground/break\|fix\|replay` | 30/hour/IP, burst 10 |
| `GET /api/playground/bootstrap` | 10/hour/IP, burst 3 |
| `GET /api/playground/stream` | 5 concurrent/IP |

Return **429** + `Retry-After`. Not registered when `TUMA_DEMO_MODE=false`.

---

## Frontend (`/playground`)

Add route in React app — **no login**.

```text
┌─────────────────────────────────────────────────────────────────┐
│  Simulated Stripe · Demo destination · Real Tuma stack          │
├──────────────────┬──────────────────────┬───────────────────────┤
│ LIVE STATUS      │ PROVIDER SIMULATOR   │ DESTINATION LOG       │
│ ● Delivering     │ Provider [Stripe ▼]  │ 12:01 POST /hook 200  │
│ Delivered: 3     │ [Send 1] [Send 5]    │   X-Tuma-Delivery-Id  │
│ Open issues: 0   │ [Send duplicate]     │   {"type":...}        │
│ [Break][Fix]     │ ── sim log ──        │                       │
│ [Replay]         │                      │                       │
│ Open console ↗   │                      │                       │
└──────────────────┴──────────────────────┴───────────────────────┘
```

- Log panes: monospace `<pre>`/div, auto-scroll — **not** xterm
- On mount: bootstrap + EventSource stream
- Link from marketing site when live
- Optional: shared demo creds for full console at `/` (Phase 2 polish; hide New connection when demo mode)

### Demo mode console guards

When `GET /api/config` returns `{ demo_mode: true }`:

- Hide "New connection"
- `POST /api/connections` → 403 outside playground bootstrap

---

## Config & deploy (demo droplet)

```bash
TUMA_DEMO_MODE=true
TUMA_PUBLIC_BASE_URL=https://tuma-demo.koto7.dev
TUMA_PLAYGROUND_SESSION_TTL_HOURS=2
TUMA_PLAYGROUND_MAX_SESSIONS=500
TUMA_PLAYGROUND_INTERNAL_BASE=http://tuma-api:8080
```

### Caddyfile (in repo — not hand-edited on server)

```caddy
tuma-demo.koto7.dev {
    reverse_proxy /api/* tuma-api:8080
    reverse_proxy /e/* tuma-api:8080
    reverse_proxy /playground/r/* tuma-api:8080
    reverse_proxy /healthz tuma-api:8080
    reverse_proxy /readyz tuma-api:8080
    reverse_proxy tuma-web:80
}
```

Do **not** expose `/metrics` publicly (Prometheus scrape stays internal).

---

## Playground implementation phases

### 2a — Backend foundation (~2–3 days)

- [ ] Migrations: `playground_sessions`, `connections.is_playground`
- [ ] `TUMA_DEMO_MODE` config
- [ ] Bootstrap, status, stream, virtual receiver
- [ ] Per-session connection + short retry policy
- [ ] GC goroutine
- [ ] Rate limiter middleware
- [ ] Filter playground from normal list API
- [ ] Tests: bootstrap, break/fix, 429, GC, two-session isolation

### 2b — Simulator (~1 day)

- [ ] `internal/playground/simulate.go`
- [ ] Stripe, GitHub, EasyPost, Internal providers
- [ ] SSE sim log wiring

### 2c — UI (~2 days)

- [ ] `PlaygroundPage.tsx` — 3-pane layout
- [ ] Route `/playground`, bootstrap + SSE + polling
- [ ] Mobile stack layout
- [ ] 429 / session-expired re-bootstrap

### 2d — Polish & ship (~1 day)

- [ ] Break → Issues → Fix → Replay loop < 60s
- [ ] Dedup demo button
- [ ] Two browser profiles simultaneously — no cross-talk
- [ ] Link on tuma.koto7.io: "Try live playground →"
- [ ] README demo URL section

**Playground estimated effort:** ~6–8 dev days after EasyPost is done.

---

## Playground ship checklist

- [ ] Visitor A Breaks; Visitor B still Delivering (two profiles, same time)
- [ ] Rate limit: scripted loop gets 429
- [ ] GC removes idle session after TTL
- [ ] EasyPost option in simulator works
- [ ] HTTPS on demo URL
- [ ] CI deploy green

---

# Phase 3 — Outreach (after playground live)

### Marketing

- [ ] tuma.koto7.io hero secondary link → `https://tuma-demo.koto7.dev/playground`
- [ ] README top: live playground URL + one-liner
- [ ] Optional: shared demo console creds on playground page only

### Cold email one-liner

> Try it: tuma-demo.koto7.dev/playground — simulated provider on the left, real Tuma in the middle, destination log on the right. Break the destination, watch Issues, replay — ~30 seconds, no signup.

### EasyPost / logistics angle

> Same stack for EasyPost tracking webhooks — self-hosted, your Postgres, MIT.

---

# Explicit non-goals

- Shared singleton playground connection
- iframe console embed
- Unauthenticated connection creation in main console
- Real terminal emulator in browser
- Terraform / Helm / K8s before outreach
- Basic-auth EasyPost webhooks (v1)
- Legacy EasyPost HMAC scheme (v1)

---

# Timeline (suggested)

```text
Week 1     Phase 1: EasyPost adapter + Test-mode verification
Week 2     Phase 2a–2b: Playground backend + simulator
Week 3     Phase 2c–2d: Playground UI + polish + marketing link
Week 4     Cold outreach + HN (when checklist green)
```

Adjust based on EasyPost Test account access and signature port difficulty.

---

# Related docs

| Doc | Purpose |
|---|---|
| `docs/tuma-technical-design.md` | Core architecture |
| `docs/tuma-cursor-build-brief.md` | Original v1 build order |
| `README.md` | Team test plan, quickstart |
| `docs/PRODUCTION.md` | *(to write)* Single-node vs split vs k8s notes |

---

# Reference: EasyPost adapter source doc

This plan incorporates `tuma-easypost-adapter-design.md` (2026-08-28). The adapter section above is the executable checklist derived from that design.
