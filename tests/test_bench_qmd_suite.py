from tests import bench_qmd_suite as suite


def test_generate_docs_creates_realistic_chat_and_session_corpus():
    docs = suite.generate_docs(files=24, target_bytes=24 * 8192, seed=42)

    relpaths = [doc.relpath for doc in docs]
    contents = [doc.content for doc in docs]

    assert len(docs) == 24
    assert any(path.endswith(".md") for path in relpaths)
    assert any(path.endswith(".log") for path in relpaths)
    assert any("sessions/agent" in path for path in relpaths)
    assert any("chat/support" in path for path in relpaths)
    assert any("memories/agents" in path for path in relpaths)
    assert any("incidents" in path for path in relpaths)

    joined = "\n".join(contents)
    assert "## Conversation" in joined
    assert "## Session Summary" in joined
    assert "## Incident Timeline" in joined
    assert "auth token refresh failed after queue timeout" in joined
    assert "disk full recovery note appended by agent" in joined
    assert "memory checkpoint persisted for follow up" in joined
    assert max(len(doc.content.encode("utf-8")) for doc in docs) >= 8192


def test_build_search_cases_include_full_and_path_scoped_queries():
    cases = suite.build_search_cases("bench-20260402-120000")

    by_name = {case.name: case for case in cases}

    assert {"auth-timeout", "disk-full", "checkpoint-sessions"} <= set(by_name)
    assert by_name["auth-timeout"].grep_scope == "bench"
    assert by_name["auth-timeout"].qmd_mode == "query"
    assert 'path:/bench-20260402-120000/bench' in by_name["auth-timeout"].qmd_query
    assert 'path:/bench-20260402-120000/bench' in by_name["disk-full"].qmd_query
    assert by_name["checkpoint-sessions"].grep_scope == "bench/sessions/agent"
    assert '"memory checkpoint persisted for follow up"' in by_name["checkpoint-sessions"].qmd_query
    assert 'path:/bench-20260402-120000/bench/sessions/agent' in by_name["checkpoint-sessions"].qmd_query


def test_compare_search_results_reports_backend_differences():
    case = suite.SearchCase(
        name="auth-timeout",
        description="rare auth timeout phrase",
        grep_pattern="auth token refresh failed after queue timeout",
        grep_scope="bench",
        qmd_mode="query",
        qmd_query='"auth token refresh failed after queue timeout"',
    )

    comparison = suite.compare_search_results(
        case,
        local_paths=["bench/a.md", "bench/b.md"],
        mount_paths=["bench/a.md", "bench/b.md"],
        qmd_paths=["bench/a.md"],
    )

    assert comparison.local_vs_mount_match is True
    assert comparison.local_vs_qmd_match is False
    assert comparison.local_count == 2
    assert comparison.mount_count == 2
    assert comparison.qmd_count == 1
    assert comparison.missing_from_qmd == ("bench/b.md",)
    assert comparison.extra_in_qmd == ()


def test_bench_files_visible_requires_expected_count():
    status = suite.BenchFlushStatus(file_count=3, missing_path_count=9)
    assert suite.bench_files_visible(status, expected_files=3) is True

    status = suite.BenchFlushStatus(file_count=2, missing_path_count=0)
    assert suite.bench_files_visible(status, expected_files=3) is False

    status = suite.BenchFlushStatus(file_count=3, missing_path_count=1)
    assert suite.bench_files_visible(status, expected_files=3) is True


def test_bench_paths_backfilled_requires_no_missing_paths():
    status = suite.BenchFlushStatus(file_count=3, missing_path_count=0)
    assert suite.bench_paths_backfilled(status) is True

    status = suite.BenchFlushStatus(file_count=3, missing_path_count=1)
    assert suite.bench_paths_backfilled(status) is False
