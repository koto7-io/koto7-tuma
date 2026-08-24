#!/usr/bin/env python3
"""Send a signed Stripe-style webhook POST to a Tuma inbound URL.

Usage:
  python3 scripts/simulate-stripe-event.py \\
    --url http://localhost/e/YOUR_INBOUND_PATH \\
    --secret whsec_... \\
    [--type invoice.payment_failed] \\
    [--dest-down]   # optional: also POST to destination to show delivery

No Stripe account required — generates a valid Stripe-Signature header.
"""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import sys
import time
import urllib.error
import urllib.request
import uuid


def stripe_signature(secret: str, body: bytes, ts: int | None = None) -> str:
    t = ts if ts is not None else int(time.time())
    payload = f"{t}.".encode() + body
    sig = hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    return f"t={t},v1={sig}"


def sample_event(event_type: str) -> dict:
    eid = f"evt_sim_{uuid.uuid4().hex[:16]}"
    return {
        "id": eid,
        "object": "event",
        "type": event_type,
        "livemode": True,
        "created": int(time.time()),
        "data": {
            "object": {
                "id": f"in_sim_{uuid.uuid4().hex[:8]}",
                "object": "invoice",
                "amount_due": 4900,
                "currency": "usd",
                "customer": "cus_sim_test",
            }
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
    p = argparse.ArgumentParser(description="Simulate a Stripe webhook to Tuma")
    p.add_argument("--url", required=True, help="Tuma inbound URL, e.g. http://localhost/e/8f3a2c91")
    p.add_argument("--secret", required=True, help="Signing secret from connection create (whsec_...)")
    p.add_argument("--type", default="invoice.payment_failed", help="Stripe event type")
    p.add_argument("--event-id", help="Fixed provider event id (default: random, for dedup tests re-run same id)")
    p.add_argument("--count", type=int, default=1, help="Number of events to send (default: 1, each gets a new id unless --event-id set)")
    args = p.parse_args()

    ok = 0
    for n in range(args.count):
        event = sample_event(args.type)
        if args.event_id:
            event["id"] = args.event_id if args.count == 1 else f"{args.event_id}_{n + 1}"
        elif args.count > 1:
            pass  # sample_event already randomizes id

        body = json.dumps(event, separators=(",", ":")).encode()
        sig = stripe_signature(args.secret, body)

        print(f"POST {args.url}")
        print(f"  event.id = {event['id']}")
        print(f"  type     = {event['type']}")

        status, text = post(
            args.url,
            body,
            {
                "Content-Type": "application/json",
                "Stripe-Signature": sig,
                "User-Agent": "Tuma-Stripe-Simulator/1.0",
            },
        )
        print(f"  → {status} {text}\n")

        if status != 200:
            print("Failed. Check: connection exists, secret matches, Tuma stack is up.", file=sys.stderr)
            return 1
        ok += 1

    print(f"Sent {ok} event(s). Check Recent deliveries / Issues in the UI.")
    if args.count == 1:
        print("Re-run with --event-id", event["id"], "to test duplicate dedup (should still 200, no second workflow).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
