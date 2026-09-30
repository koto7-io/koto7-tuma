# Temporal operations

Tuma uses Temporal for durable retries. `docker compose up` starts it for you. You are still running Temporal.

Validated image, pinned in `deploy/docker-compose.yml`: **`temporalio/auto-setup:1.25.2`**. CI does not boot Temporal. Move one minor release at a time and read the [Temporal server upgrade notes](https://docs.temporal.io/self-hosted-guide/upgrade-server) before you bump the tag. Skipping two minor versions is how schema migrations fail in the middle.

## Upgrade

1. Stop the worker: `docker compose stop tuma-worker`. Leave Postgres and the API up.
2. Change the `temporal` image tag in `deploy/docker-compose.yml`.
3. `docker compose up -d temporal`. The auto-setup image applies Temporal's own schema migrations.
4. Start the worker again: `docker compose up -d tuma-worker`.
5. Smoke test: send one signed event, see a delivery, then fail a destination and replay the issue.

Tuma's SQL is separate. Deploying a new `tuma` binary runs `golang-migrate` from the API process only. Bumping the Temporal image does not run those files, and deploying Tuma does not migrate Temporal.

## One Postgres, two schemas

The default Compose file uses a single Postgres for both.

| | Temporal | Tuma |
|---|---|---|
| Schema owner | `temporalio/auto-setup` | `migrations/` via the API process |
| Operator action | Bump the image, as above | Deploy a new `tuma` binary |

They do not share tables or migration tooling.

## What v1 does not need

Temporal's gRPC port and Web UI stay on the Docker network. No host ports. You do not need `tctl`, the Temporal Web UI, or a custom task queue to run Connections, Issues, or replay.

Workflow history sits in that same Postgres volume. Tuma deletes events past each connection's `retention_days` every hour. History Temporal still holds is the rest of the disk growth. v1 does not expose a control for it.

## Workers

More than one `tuma-worker` is safe for delivery. Temporal hands each activity to one worker. Alert checks take a Postgres advisory lock, so only one worker sends a given alert.
