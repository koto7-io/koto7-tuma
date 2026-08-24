#!/usr/bin/env python3
"""Minimal webhook receiver for local Tuma destination testing.

Usage:
  python3 scripts/webhook-echo.py [--port 9999] [--fail]

Prints each POST. Use --fail to return 502 (triggers Tuma retries → Issues).

Example Tuma destination URL: http://host.docker.internal:9999/hook
  (from Docker Desktop on Mac; on Linux use your machine IP instead)
"""

from __future__ import annotations

import argparse
import json
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    fail: bool = False

    def do_POST(self) -> None:
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        delivery_id = self.headers.get("X-Tuma-Delivery-Id", "—")
        attempt = self.headers.get("X-Tuma-Attempt", "—")
        print("\n--- delivery ---")
        print(f"  X-Tuma-Delivery-Id: {delivery_id}")
        print(f"  X-Tuma-Attempt:     {attempt}")
        try:
            print(json.dumps(json.loads(body), indent=2))
        except json.JSONDecodeError:
            print(body.decode(errors="replace"))

        if self.fail:
            self.send_response(502)
            self.end_headers()
            self.wfile.write(b"simulated failure")
            print("  → responded 502")
        else:
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            print("  → responded 200")

    def log_message(self, *_: object) -> None:
        pass


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--port", type=int, default=9999)
    p.add_argument("--fail", action="store_true", help="Always return 502")
    args = p.parse_args()
    Handler.fail = args.fail
    print(f"Listening on http://0.0.0.0:{args.port}/ (any path)")
    print("Tuma destination (Docker Desktop Mac): http://host.docker.internal:{}/hook".format(args.port))
    HTTPServer(("0.0.0.0", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
