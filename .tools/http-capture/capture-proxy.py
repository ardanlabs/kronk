#!/usr/bin/env python3
"""Capture HTTP request bodies while streaming responses from an upstream server."""

import argparse
import http.client
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


HOP_BY_HOP_HEADERS = {
    "connection",
    "content-length",
    "keep-alive",
    "proxy-authenticate",
    "proxy-authorization",
    "te",
    "trailer",
    "transfer-encoding",
    "upgrade",
}


class CaptureState:
    def __init__(self, output_dir: Path):
        self.output_dir = output_dir
        self.lock = threading.Lock()
        self.sequence = 0

    def save(self, method: str, path: str, headers, body: bytes) -> Path:
        with self.lock:
            self.sequence += 1
            sequence = self.sequence

        stem = f"request-{sequence:04d}"
        body_path = self.output_dir / f"{stem}.json"
        metadata_path = self.output_dir / f"{stem}.headers.json"
        body_path.write_bytes(body)
        metadata_path.write_text(
            json.dumps(
                {
                    "method": method,
                    "path": path,
                    "headers": dict(headers.items()),
                },
                indent=2,
                sort_keys=True,
            )
            + "\n"
        )
        return body_path


class CaptureProxyHandler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        self._forward(b"")

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        body_path = self.server.capture_state.save(
            self.command, self.path, self.headers, body
        )
        print(f"captured {body_path} ({len(body)} bytes)", flush=True)
        self._forward(body)

    def _forward(self, body: bytes):
        connection = http.client.HTTPConnection(
            self.server.upstream_host,
            self.server.upstream_port,
            timeout=600,
        )
        headers = {
            name: value
            for name, value in self.headers.items()
            if name.lower() not in HOP_BY_HOP_HEADERS and name.lower() != "host"
        }
        if body:
            headers["Content-Length"] = str(len(body))

        try:
            connection.request(self.command, self.path, body=body, headers=headers)
            response = connection.getresponse()
            self.send_response(response.status, response.reason)
            for name, value in response.getheaders():
                if name.lower() not in HOP_BY_HOP_HEADERS:
                    self.send_header(name, value)
            self.send_header("Connection", "close")
            self.end_headers()

            while chunk := response.read1(64 * 1024):
                self.wfile.write(chunk)
                self.wfile.flush()
        except BrokenPipeError:
            pass
        except Exception as exc:
            if not self.wfile.closed:
                self.send_error(502, f"upstream request failed: {exc}")
        finally:
            self.close_connection = True
            connection.close()

    def log_message(self, format, *args):
        print(f"proxy: {format % args}", flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--listen-host", default="127.0.0.1")
    parser.add_argument("--listen-port", default=11436, type=int)
    parser.add_argument("--upstream-host", default="127.0.0.1")
    parser.add_argument("--upstream-port", default=11435, type=int)
    parser.add_argument("--output-dir", default=".tools/http-capture/output")
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    server = ThreadingHTTPServer(
        (args.listen_host, args.listen_port), CaptureProxyHandler
    )
    server.capture_state = CaptureState(output_dir)
    server.upstream_host = args.upstream_host
    server.upstream_port = args.upstream_port

    print(
        f"capturing http://{args.listen_host}:{args.listen_port} -> "
        f"http://{args.upstream_host}:{args.upstream_port} in {output_dir}",
        flush=True,
    )
    server.serve_forever()


if __name__ == "__main__":
    main()
