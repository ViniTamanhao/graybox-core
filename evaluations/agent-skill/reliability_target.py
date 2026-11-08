#!/usr/bin/env python3
"""Synthetic local failure target; no external integrations or real credentials."""
import argparse
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from pathlib import Path


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        self.respond()

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.respond()

    def respond(self):
        server = self.server
        with server.ledger.open("a") as out:
            out.write(json.dumps({"method": self.command, "path": self.path}) + "\n")
        if self.command == "POST":
            server.writes += 1
            server.counter.write_text(str(server.writes))
        live = server.mode == "live"
        if live and self.path == "/write":
            # State committed, then no HTTP response: the caller cannot know this.
            self.close_connection = True
            return
        partial = live and self.path == "/failed"
        status = int(self.path[8:]) if self.path.startswith("/status/") else 201 if partial else 200
        body = json.dumps({"value": 2 if live and self.path == "/changed" else 1}).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body) + (20 if partial else 0)))
        self.end_headers()
        self.wfile.write(body)
        self.wfile.flush()
        self.close_connection = True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--mode", choices=("reference", "live"), default="live")
    parser.add_argument("--ledger", type=Path, required=True)
    parser.add_argument("--counter", type=Path, required=True)
    args = parser.parse_args()
    with HTTPServer(("127.0.0.1", 0), Handler) as server:
        server.mode, server.ledger, server.counter = args.mode, args.ledger, args.counter
        server.writes = 0
        server.counter.write_text("0")
        print("http://127.0.0.1:" + str(server.server_port), flush=True)
        server.serve_forever()


if __name__ == "__main__":
    main()
