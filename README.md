# Tuma — team guide

**Website:** https://tuma.koto7.io · **License:** MIT

Self-hosted webhook reliability layer. Providers POST to Tuma; Tuma stores events durably, delivers to your app with retries, and surfaces failures for replay.

**Console:** Connections · Metrics · Issues  
**Stack:** Go API + Temporal worker + Postgres + React UI (Docker Compose)

---

## Quickstart (local)

```bash
cd deploy
docker compose up --build
```

| URL | Purpose |
|---|---|
| http://localhost | Tuma console (Connections, Metrics, Issues) |
| http://localhost:3001 | Grafana (Prometheus dashboards — separate login) |

### First login

On **first boot only**, the admin password is auto-generated:

```bash
docker compose logs tuma-api | grep -A3 "TUMA FIRST BOOT"
```

```
=== TUMA FIRST BOOT ===
  Email:    admin@localhost
  Password: <generated>
```

**Save that password immediately.** It is logged once and not shown again.

To pin a known password on a **fresh** install (before any data):

```bash
# deploy/.env
TUMA_ADMIN_EMAIL=you@company.com
TUMA_ADMIN_PASSWORD='use-a-long-random-string'
```

Then `docker compose up --build`. If you already booted without setting this, the generated password is what you have — or wipe the volume (`docker compose down -v`) and start clean.

**Dark mode:** sidebar → **Theme · Light/Dark/System** (persists in browser). Login page has a toggle top-right.

---

## Security — read before sharing with the team

The default Compose file is for **local dev**. Treat these as public if you expose the host:

| Secret / surface | Default (dev) | What to do |
|---|---|---|
| **TUMA admin password** | Auto-generated or `TUMA_ADMIN_PASSWORD` | Set a strong password in `deploy/.env` before first boot in shared/staging/prod. Store in your password manager — not Slack. |
| **TUMA_ENCRYPTION_KEY** | Hardcoded 32-byte demo key in compose | Generate a unique key per environment. **Never rotate** without re-entering all connection signing secrets. |
| **Postgres** | `tuma` / `tuma` on internal network only | Use strong `POSTGRES_PASSWORD` + `DATABASE_URL` for anything beyond localhost. |
| **Grafana** | `admin` / `tuma` on **:3001** | Change `GF_SECURITY_ADMIN_PASSWORD`. Consider not publishing `:3001` to the internet (VPN / SSH tunnel). |
| **Signing secrets** | Shown once when creating a connection | Copy into Stripe/GitHub dashboard; Tuma encrypts at rest. Don't commit secrets to git. |
| **Inbound webhooks** (`POST /e/{path}`) | Public by design | URLs are unguessable paths, not secret. **Verification is the signing secret** (Stripe HMAC, etc.). `internal` source skips verification — use only on trusted networks. |
| **Session cookies** | HttpOnly, SameSite=Lax, 7-day TTL | `Secure` flag is set automatically when `TUMA_PUBLIC_BASE_URL` starts with `https://`. Terminate TLS at Caddy/reverse proxy. |
| **Temporal / Prometheus** | Internal Docker network only | Not exposed on host ports in default compose — keep it that way. |

### Production checklist

1. Copy `deploy/.env.example` → `deploy/.env` and fill in all required values.
2. Set `TUMA_PUBLIC_BASE_URL=https://hooks.yourdomain.com` (matches TLS termination).
3. Generate `TUMA_ENCRYPTION_KEY`: `openssl rand -base64 24 | head -c 32`
4. Set `TUMA_ADMIN_PASSWORD` before first boot (or retrieve generated password from logs once).
5. Change Postgres and Grafana passwords.
6. Restrict network access (firewall, private VPC, no public `:3001`).
7. Enable Postgres backups (see below).
8. Tell destination app teams: **handlers must be idempotent** (see At-least-once).

---

## Console tour

### Connections

Create a source → destination pipe. Each connection gets a unique inbound URL and (for signed sources) a signing secret.

Card stats (last 24h): **Delivered**, **p95 latency**, **Open issues**. Status pill:

| Pill | Meaning |
|---|---|
| Delivering | No open issues; healthy |
| Degraded | Failed delivery attempts in last 24h, but nothing stuck in Issues |
| Failing | Open issues — retries exhausted, needs replay or destination fix |

### Metrics

Platform-wide 24h view: ingestion, deliveries, failures, p95, hourly charts, per-connection breakdown. Refreshes every 30s. Grafana on `:3001` is for deeper Prometheus/Temporal ops.

### Issues

Failed deliveries after all retries. Open an issue → inspect payload → **Replay** after fixing the destination. Bulk-select + **Replay selected** for batches.

---

## Team test plan

Run through this before go-live. No Stripe account required for most tests.

### Setup: echo destination

Terminal 1 — receives what Tuma delivers outbound:

```bash
python3 scripts/webhook-echo.py --port 9999
```

In Tuma, create a connection with **Destination URL**:

- **Docker Desktop (Mac/Windows):** `http://host.docker.internal:9999/hook`
- **Linux:** `http://<your-lan-ip>:9999/hook`

### Test 1 — Happy path (Stripe adapter)

1. Create a **Stripe** connection. Copy **Webhook URL** and **Signing secret** (the `whsec_…` Tuma shows — not a placeholder).
2. Send signed events:

```bash
python3 scripts/simulate-stripe-event.py \
  --url http://localhost/e/YOUR_INBOUND_PATH \
  --secret whsec_REAL_FROM_UI \
  --count 5
```

3. **Expect:** simulator prints `→ 200 {"received":true}`; echo server logs each delivery; connection shows Delivered count + **Delivering** pill; Metrics charts update; Recent deliveries show `delivered` + latency.

### Test 2 — Failure → Issues → Replay

1. Stop echo server. Restart with permanent failure:

```bash
python3 scripts/webhook-echo.py --port 9999 --fail
```

2. Send another simulated event (new event id — omit `--event-id` or use a new one).
3. **Expect:** after retries exhaust, issue appears in **Issues** (Open); connection pill → **Failing**.
4. Restart echo **without** `--fail`. In Issues, open the issue → **Replay event**.
5. **Expect:** delivery succeeds; issue moves to Resolved; echo server receives POST.

### Test 3 — Dedup (inbound)

Same provider event id twice = one workflow:

```bash
python3 scripts/simulate-stripe-event.py --url ... --secret ... --event-id evt_dedup_test_1
python3 scripts/simulate-stripe-event.py --url ... --secret ... --event-id evt_dedup_test_1
```

**Expect:** both return 200; only **one** delivery in Recent deliveries / one echo log line.

### Test 4 — Internal source (no signature)

Create connection with source **Internal**:

```bash
curl -X POST http://localhost/e/YOUR_PATH \
  -H 'Content-Type: application/json' \
  -H 'X-Tuma-Event-Id: evt_curl_1' \
  -d '{"hello":"world"}'
```

Use for quick smoke tests only — no inbound signature verification.

### Test 5 — Destination idempotency (at-least-once)

Replay and retries can duplicate POSTs to your app. Every outbound delivery includes:

```
X-Tuma-Delivery-Id: <uuid>
```

**Expect:** your destination dedupes on that header (or a business key in the payload).

### Test 6 — Metrics & status pills

After tests 1–3, verify Metrics page totals match connection stats. Toggle pills between Delivering / Degraded / Failing by succeeding vs failing deliveries.

---

## Testing with real EasyPost

1. Create an **EasyPost** connection; set destination to your app or https://webhook.site/…
2. In EasyPost Dashboard → Webhooks, paste Tuma **Webhook URL** and use the **webhook secret** when creating the connection.
3. Send a test event from EasyPost (Test mode) → check Recent deliveries.
4. Tuma verifies legacy **`X-Hmac-Signature`** (HMAC-SHA256 over raw body — matches official EasyPost SDKs). After deploy, confirm the header EasyPost sends; if you see `x-hmac-signature-v2` instead, open an issue with captured headers.

Local smoke test (no EasyPost account):

```bash
python3 scripts/simulate-easypost-event.py \
  --url https://tuma-demo.koto7.dev/e/YOUR_PATH \
  --secret YOUR_WEBHOOK_SECRET
```

---

## Testing with real Stripe

1. Create Stripe connection; set destination to your app or https://webhook.site/…
2. Paste Tuma **Webhook URL** into Stripe Dashboard → Developers → Webhooks.
3. Use the signing secret from Tuma when creating the connection.
4. Send test event from Stripe → check Recent deliveries.
5. Break destination → trigger events → Issues → fix → Replay.

---

## At-least-once delivery

Tuma guarantees **at-least-once**, not exactly-once. Retries and replays can cause duplicate POSTs.

**Your handler must be idempotent.** Dedupe on `X-Tuma-Delivery-Id` or your own business key.

---

## Configuration reference

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | compose internal | Postgres |
| `TEMPORAL_HOST` | `temporal:7233` | Temporal frontend |
| `TUMA_ENCRYPTION_KEY` | *(required)* | 32-byte key; encrypts signing secrets at rest |
| `TUMA_PUBLIC_BASE_URL` | `http://localhost` | Shown in webhook URLs; `https://` enables Secure cookies |
| `TUMA_ADMIN_EMAIL` | `admin@localhost` | Bootstrap admin (first boot only) |
| `TUMA_ADMIN_PASSWORD` | *(generated)* | Set before first boot to pin password |
| `TUMA_SESSION_TTL_HOURS` | `168` | Console session lifetime |
| `TUMA_PER_CONN_CONCURRENCY` | `10` | Max concurrent deliveries per connection |
| `TUMA_MAX_BODY_BYTES` | `1048576` | Max inbound webhook body size |

See `deploy/.env.example` for production template.

---

## Backup & restore

Postgres holds all events, deliveries, issues, and encrypted secrets. Temporal state is also in Postgres in the default stack.

```bash
# Logical backup
docker compose exec postgres pg_dump -U tuma tuma > tuma-$(date +%F).sql

# Restore
docker compose exec -T postgres psql -U tuma tuma < tuma-2026-08-24.sql
```

For production: managed Postgres with automated backups and PITR. **Issues replay covers destination outages, not database loss.**

---

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| 401 on simulator | Wrong signing secret — use exact `whsec_…` from connection create wizard |
| Login fails after rebuild | Same DB volume — password unchanged. Use saved password or check logs from original first boot |
| No deliveries in echo | Wrong destination URL (`host.docker.internal` vs LAN IP); worker not running |
| API container restart loop | Temporal not ready yet — wait; `restart: unless-stopped` should recover |
| Session lost on HTTPS | Set `TUMA_PUBLIC_BASE_URL` to `https://…` matching your public URL |

**Fresh start (wipes all data):**

```bash
cd deploy && docker compose down -v && docker compose up --build
```

---

## Development (without Docker)

```bash
export TUMA_ENCRYPTION_KEY='01234567890123456789012345678901'
export DATABASE_URL='postgres://tuma:tuma@localhost:5432/tuma?sslmode=disable'

go run ./cmd/tuma serve api      # terminal 1
go run ./cmd/tuma serve worker   # terminal 2
cd web && npm install && npm run dev  # terminal 3
```

---

## Architecture & scope

- **v1 included:** connections, retries, issues/DLQ + replay, session auth, in-app metrics, Prometheus/Grafana, dark mode
- **Not in v1:** multi-tenancy, billing, RBAC, transformations

---

## Scripts

| Script | Purpose |
|---|---|
| `scripts/webhook-echo.py` | Local destination; `--fail` for 502 |
| `scripts/simulate-stripe-event.py` | Signed Stripe-style inbound; `--count N`, `--event-id` |
| `scripts/simulate-easypost-event.py` | Signed EasyPost-style inbound (legacy `X-Hmac-Signature`); `--count N`, `--event-id` |
