#!/usr/bin/env python3
"""Send a signed EasyPost-style webhook POST to a Tuma inbound URL.

Uses the legacy X-Hmac-Signature scheme (HMAC-SHA256 over raw body) matching
EasyPost's official Python/Node validate_webhook helpers.

Usage:
  python3 scripts/simulate-easypost-event.py \\
    --url https://tuma-demo.koto7.dev/e/YOUR_INBOUND_PATH \\
    --secret YOUR_WEBHOOK_SECRET \\
    [--description tracker.updated] \\
    [--event-id evt_fixed] \\
    [--count 5]

No EasyPost account required for local smoke tests — generates a valid signature.
Verify against a real Test-mode webhook after deploy to confirm header format.
"""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import sys
import unicodedata
import urllib.error
import urllib.request
import uuid


def easypost_signature(secret: str, body: bytes) -> str:
    normalized = unicodedata.normalize("NFKD", secret)
    digest = hmac.new(normalized.encode("utf-8"), body, hashlib.sha256).hexdigest()
    return f"hmac-sha256-hex={digest}"


def sample_event(description: str, event_id: str | None) -> dict:
    eid = event_id or f"evt_sim_{uuid.uuid4().hex[:16]}"
    return {
        "id": eid,
        "object": "Event",
        "mode": "test",
        "description": description,
        "created_at": "2026-01-15T12:00:00Z",
        "result": {
            "id": f"trk_{uuid.uuid4().hex[:12]}",
            "object": "Tracker",
            "tracking_code": "9400111899223344556677",
            "status": "in_transit",
        },
    }


def post(url: str, body: bytes, headers: dict[str, str]) -> tuple[int, str]:
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            return resp.status, resp.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def main() -> int:
    p = argparse.ArgumentParser(description="Simulate an EasyPost webhook to Tuma")
    p.add_argument("--url", required=True, help="Tuma inbound URL")
    p.add_argument("--secret", required=True, help="Webhook secret from EasyPost / connection create")
    p.add_argument("--description", default="tracker.updated", help="EasyPost event description")
    p.add_argument("--event-id", help="Fixed provider event id (dedup tests)")
    p.add_argument("--count", type=int, default=1, help="Events to send")
    args = p.parse_args()

    ok = 0
    last_id = ""
    for n in range(args.count):
        eid = args.event_id
        if eid and args.count > 1:
            eid = f"{args.event_id}_{n + 1}"
        event = sample_event(args.description, eid)
        last_id = event["id"]

        body = json.dumps(event, separators=(",", ":")).encode()
        sig = easypost_signature(args.secret, body)

        print(f"POST {args.url}")
        print(f"  event.id      = {event['id']}")
        print(f"  description   = {event['description']}")
        print(f"  X-Hmac-Signature = {sig[:40]}...")

        status, text = post(
            args.url,
            body,
            {
                "Content-Type": "application/json",
                "X-Hmac-Signature": sig,
                "User-Agent": "Tuma-EasyPost-Simulator/1.0",
            },
        )
        print(f"  → {status} {text}\n")

        if status != 200:
            print("Failed. Check secret, connection source_type=easypost, stack up.", file=sys.stderr)
            return 1
        ok += 1

    print(f"Sent {ok} event(s). Check Recent deliveries / Metrics in the UI.")
    if args.count == 1 and last_id:
        print(f"Re-run with --event-id {last_id} to test dedup.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
