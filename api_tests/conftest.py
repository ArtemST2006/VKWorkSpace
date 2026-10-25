"""Black-box fixtures for the ghreport executable."""

import json
import os
import subprocess
import threading
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlsplit

import pytest


ROOT = Path(__file__).resolve().parents[1]


@dataclass
class Request:
    path: str
    query: dict[str, list[str]]
    headers: dict[str, str]


class Stub:
    def __init__(self):
        self.base_url = ""
        self.requests: list[Request] = []
        self.dispatch = lambda _request: (404, {}, {"message": "not found"})


@pytest.fixture
def github_stub():
    """A fresh GitHub-compatible read-only API for each test."""
    stub = Stub()

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            address = urlsplit(self.path)
            headers = {name.lower(): value for name, value in self.headers.items()}
            request = Request(address.path, parse_qs(address.query), headers)
            stub.requests.append(request)
            status, headers, body = stub.dispatch(request)
            payload = body if isinstance(body, bytes) else json.dumps(body).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(payload)))
            for name, value in headers.items():
                self.send_header(name, value)
            self.end_headers()
            self.wfile.write(payload)

        def log_message(self, _format, *_args):
            pass

    server = HTTPServer(("127.0.0.1", 0), Handler)
    stub.base_url = f"http://127.0.0.1:{server.server_port}"
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        yield stub
    finally:
        server.shutdown()
        worker.join(timeout=2)
        server.server_close()


@pytest.fixture(scope="session")
def ghreport_bin():
    path = Path(os.environ.get("GHREPORT_BIN", ROOT / "service/bin/ghreport")).resolve()
    if not path.is_file():
        pytest.fail(f"ghreport binary not found: {path}; build it or set GHREPORT_BIN")
    return path


@pytest.fixture
def run_ghreport(ghreport_bin, github_stub):
    """Launch ghreport as a separate process against this test's API."""
    def run(repos, cache_dir, *args, timeout=15):
        env = os.environ.copy()
        env.update(
            GITHUB_API_BASE_URL=github_stub.base_url,
            GITHUB_TOKEN="integration-test-token",
            HTTP_PROXY="", HTTPS_PROXY="", ALL_PROXY="", NO_PROXY="*",
        )
        command = [str(ghreport_bin), "--repos", repos, "--out", "-",
                   "--cache-dir", str(cache_dir), *args]
        return subprocess.run(command, capture_output=True, text=True,
                              env=env, timeout=timeout, check=False)

    return run
