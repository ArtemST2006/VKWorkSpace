"""Observable ghreport behaviour against a local API stub."""

import json
import time
from datetime import datetime
from urllib.parse import urlencode


TRACE_FIELDS = {"ts", "method", "url", "status", "attempt", "from_cache", "error"}
ISSUE_FIELDS = {"number", "title", "author", "labels", "created_at", "url"}


def issue(number, created_at):
    return {
        "number": number, "title": f"Issue {number}",
        "user": {"login": "octocat"}, "labels": [{"name": "bug"}],
        "created_at": created_at, "html_url": f"https://example.test/issues/{number}",
    }


def report_of(process):
    assert process.returncode == 0, process.stderr
    return json.loads(process.stdout)


def trace_of(path):
    return [json.loads(line) for line in path.read_text().splitlines()]


def test_pagination_filters_schema_and_trace(github_stub, run_ghreport, tmp_path):
    def dispatch(request):
        if request.query.get("page") == ["2"]:
            return 200, {}, [
                issue(3, "2026-01-02T00:00:00Z"),  # duplicate from page 1
                issue(2, "2026-01-02T00:00:00Z"),
                issue(4, "2026-01-03T00:00:00Z"),
                issue(5, "2026-01-04T00:00:00Z"),
            ]
        next_query = dict(request.query)
        next_query["page"] = ["2"]
        next_url = f"{github_stub.base_url}{request.path}?{urlencode(next_query, doseq=True)}"
        return 200, {"Link": f'<{next_url}>; rel="next"'}, [
            issue(1, "2025-12-31T23:59:59Z"),
            {"pull_request": {}},
            issue(3, "2026-01-02T00:00:00Z"),
        ]

    github_stub.dispatch = dispatch
    trace = tmp_path / "trace.jsonl"
    process = run_ghreport("stub/paged", tmp_path / "cache",
                           "--since", "2026-01-01", "--limit", "3", "--trace", str(trace))
    report = report_of(process)
    assert set(report) == {"generated_at", "repos"}
    assert datetime.fromisoformat(report["generated_at"].replace("Z", "+00:00")).tzinfo
    assert len(report["repos"]) == 1
    repo = report["repos"][0]
    assert set(repo) == {"repo", "error", "count", "issues"}
    assert (repo["repo"], repo["error"], repo["count"]) == ("stub/paged", None, 3)
    assert type(repo["count"]) is int and isinstance(repo["issues"], list)
    assert [item["number"] for item in repo["issues"]] == [2, 3, 4]
    for item in repo["issues"]:
        assert set(item) == ISSUE_FIELDS
        assert type(item["number"]) is int and isinstance(item["title"], str)
        assert item["author"] == "octocat" and item["labels"] == ["bug"]
        assert isinstance(item["url"], str) and isinstance(item["created_at"], str)
        assert datetime.fromisoformat(item["created_at"].replace("Z", "+00:00")).tzinfo
    assert len(github_stub.requests) == 2
    first, second = github_stub.requests
    assert first.query == {
        "state": ["open"], "sort": ["created"],
        "direction": ["asc"], "per_page": ["100"],
    }
    assert second.query["page"] == ["2"]
    assert first.headers["accept"] == "application/vnd.github+json"
    assert first.headers["x-github-api-version"] == "2026-03-10"
    assert first.headers["authorization"] == "Bearer integration-test-token"
    records = trace_of(trace)
    assert len(records) == 2
    for record in records:
        assert set(record) == TRACE_FIELDS
        assert record["method"] == "GET" and record["status"] == 200
        assert record["attempt"] == 1 and record["from_cache"] is False
        assert record["error"] is None and record["url"].startswith(github_stub.base_url)
        assert datetime.fromisoformat(record["ts"].replace("Z", "+00:00")).tzinfo
    assert "integration-test-token" not in trace.read_text() + process.stdout + process.stderr
    assert "Authorization" not in trace.read_text()


def test_etag_cache_across_processes(github_stub, run_ghreport, tmp_path):
    def dispatch(request):
        page = request.query.get("page", ["1"])[0]
        etag = f'"page-{page}"'
        if request.headers.get("if-none-match") == etag:
            return 304, {}, b""
        headers = {"ETag": etag}
        if page == "1":
            next_query = dict(request.query)
            next_query["page"] = ["2"]
            next_url = f"{github_stub.base_url}{request.path}?{urlencode(next_query, doseq=True)}"
            headers["Link"] = f'<{next_url}>; rel="next"'
        return 200, headers, [issue(int(page), f"2026-01-0{page}T00:00:00Z")]

    github_stub.dispatch = dispatch
    cache = tmp_path / "cache"
    trace = tmp_path / "trace.jsonl"
    first = report_of(run_ghreport("stub/cache", cache, "--trace", str(trace)))
    second = report_of(run_ghreport("stub/cache", cache, "--trace", str(trace)))
    assert first == second
    assert second["repos"][0]["count"] == 2
    assert len(github_stub.requests) == 4
    assert all("if-none-match" not in call.headers for call in github_stub.requests[:2])
    assert [call.headers["if-none-match"] for call in github_stub.requests[2:]] == [
        '"page-1"', '"page-2"',
    ]
    records = trace_of(trace)
    assert [record["status"] for record in records] == [304, 304]
    assert all(record["from_cache"] is True for record in records)


def test_rate_limit_waits_then_succeeds(github_stub, run_ghreport, tmp_path):
    def dispatch(_request):
        if len(github_stub.requests) == 1:
            return 429, {"Retry-After": "1"}, {"message": "rate limit exceeded"}
        return 200, {}, [issue(1, "2026-01-01T00:00:00Z")]

    github_stub.dispatch = dispatch
    trace = tmp_path / "trace.jsonl"
    started = time.monotonic()
    process = run_ghreport("stub/rate", tmp_path / "cache", "--trace", str(trace))
    elapsed = time.monotonic() - started
    assert report_of(process)["repos"][0]["count"] == 1
    assert elapsed >= 0.9
    assert len(github_stub.requests) == 2
    records = trace_of(trace)
    assert [record["status"] for record in records] == [429, 200]
    assert [record["attempt"] for record in records] == [1, 2]


def test_errors_and_exit_codes(github_stub, run_ghreport, tmp_path):
    def dispatch(request):
        if "/unauthorized/" in request.path:
            return 401, {}, {"message": "bad credentials"}
        if "/missing/" in request.path:
            return 404, {}, {"message": "not found"}
        if "/broken/" in request.path:
            return 200, {}, b"{invalid json"
        return 200, {}, []

    github_stub.dispatch = dispatch
    unauthorized = run_ghreport("stub/unauthorized", tmp_path / "unauthorized")
    assert unauthorized.returncode == 3
    assert json.loads(unauthorized.stdout) == {"error": "unauthorized"}
    missing = run_ghreport("stub/missing,stub/ok", tmp_path / "missing")
    assert missing.returncode == 1, missing.stderr
    repos = json.loads(missing.stdout)["repos"]
    assert repos[0]["error"] == "not_found"
    assert repos[0]["count"] == 0 and repos[0]["issues"] == []
    assert repos[1]["error"] is None
    broken = run_ghreport("stub/broken,stub/ok", tmp_path / "broken")
    assert broken.returncode == 1, broken.stderr
    assert json.loads(broken.stdout)["repos"][0]["error"] == "invalid_response"


def test_invalid_arguments_leave_stdout_empty(github_stub, run_ghreport, tmp_path):
    process = run_ghreport("stub/ok", tmp_path / "cache", "--limit", "0")
    assert process.returncode == 2
    assert process.stdout == "" and process.stderr
    assert github_stub.requests == []
