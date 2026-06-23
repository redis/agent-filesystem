import asyncio
import unittest
from pathlib import Path
from unittest.mock import patch

from redis_afs.errors import AFSError
from redis_afs.models import BashResult

try:
    import httpx
except ImportError:  # pragma: no cover
    httpx = None

import redis_afs.aio as aio
from redis_afs.aio import (
    AsyncAFS,
    AsyncCheckpointClient,
    AsyncFSClient,
    AsyncMCPHttpClient,
    AsyncMountedFS,
    AsyncRepoClient,
    AsyncWorkspaceClient,
    _AsyncMountedWorkspace,
)


class FakeAsyncMCP:
    def __init__(self):
        self.files = {}
        self.symlinks = {}
        self.calls = []
        self.issued = []

    async def call_tool(self, name, arguments=None):
        arguments = arguments or {}
        self.calls.append((name, dict(arguments)))
        if name == "mcp_token_issue":
            self.issued.append(dict(arguments))
            return {
                "token": f"token-{arguments['workspace']}",
                "url": "https://afs.example/mcp",
                "workspace": arguments["workspace"],
                "profile": arguments["profile"],
            }
        if name == "workspace_list":
            return {"items": [{"name": "a"}, {"name": "b"}]}
        if name == "workspace_get":
            return {"name": arguments["workspace"]}
        if name == "workspace_delete":
            return {"workspace": arguments["workspace"], "deleted": True}
        if name == "checkpoint_list":
            return {"checkpoints": [{"id": "1", "name": "head"}]}
        if name == "checkpoint_create":
            return {"workspace": arguments.get("workspace"), "checkpoint": arguments.get("checkpoint") or "auto", "created": True}
        if name == "checkpoint_restore":
            return {"workspace": arguments.get("workspace"), "checkpoint": arguments["checkpoint"], "restored": True}
        # file ops reused by later tasks
        if name == "file_write":
            self.files[arguments["path"]] = arguments["content"]
            return {"path": arguments["path"], "operation": "write"}
        if name == "file_read":
            if arguments["path"] in self.symlinks:
                return {"path": arguments["path"], "kind": "symlink", "target": self.symlinks[arguments["path"]]}
            return {"path": arguments["path"], "kind": "file", "content": self.files.get(arguments["path"], "")}
        if name == "file_list":
            return {"entries": _fake_entries(self.files, self.symlinks, arguments.get("path", "/"))}
        if name == "file_delete":
            path = arguments["path"]
            if path in self.files:
                del self.files[path]
                return {"operation": "delete", "kind": "file"}
            if path in self.symlinks:
                del self.symlinks[path]
                return {"operation": "delete", "kind": "symlink"}
            raise AssertionError(f"delete of missing path {path}")
        raise AssertionError(f"unexpected tool {name}")

    async def aclose(self):
        pass


def _fake_entries(files, symlinks, path):
    entries = []
    for p in sorted(files):
        if path == "/" and "/" not in p.strip("/"):
            entries.append({"path": p, "name": p.strip("/"), "kind": "file"})
        elif p.startswith(path.rstrip("/") + "/"):
            rem = p[len(path.rstrip("/")) + 1:]
            if "/" not in rem:
                entries.append({"path": p, "name": rem, "kind": "file"})
    for lp, target in sorted(symlinks.items()):
        if path == "/" and "/" not in lp.strip("/"):
            entries.append({"path": lp, "name": lp.strip("/"), "kind": "symlink", "target": target})
    return entries


class AsyncClientsTest(unittest.IsolatedAsyncioTestCase):
    async def test_delete_removes_file_and_calls_file_delete(self):
        fake = FakeAsyncMCP()
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="foobar", token="token", client=fake)])

        await fs.write_file("/src/README.md", "hello")
        result = await fs.delete("/foobar/src/README.md")

        self.assertEqual(result, {"operation": "delete", "kind": "file"})
        self.assertNotIn("/src/README.md", fake.files)
        delete_paths = [args["path"] for name, args in fake.calls if name == "file_delete"]
        self.assertEqual(delete_paths, ["/src/README.md"])

    async def test_workspace_list_normalizes_items(self):
        ws = AsyncWorkspaceClient(FakeAsyncMCP())
        self.assertEqual(await ws.list(), [{"name": "a"}, {"name": "b"}])

    async def test_checkpoint_round_trip(self):
        cp = AsyncCheckpointClient(FakeAsyncMCP())
        created = await cp.create(workspace="repo", checkpoint="c1")
        restored = await cp.restore(workspace="repo", checkpoint="c1")
        self.assertTrue(created["created"])
        self.assertTrue(restored["restored"])

    async def test_checkpoint_create_requires_workspace(self):
        cp = AsyncCheckpointClient(FakeAsyncMCP())
        with self.assertRaises(AFSError):
            await cp.create()

    async def test_checkpoint_restore_requires_workspace(self):
        cp = AsyncCheckpointClient(FakeAsyncMCP())
        with self.assertRaises(AFSError):
            await cp.restore(checkpoint="c1")

    async def test_workspace_get_resolves_repo_argument(self):
        ws = AsyncWorkspaceClient(FakeAsyncMCP())
        # AsyncRepoClient is an alias of AsyncWorkspaceClient.
        self.assertIs(AsyncRepoClient, AsyncWorkspaceClient)
        self.assertEqual(await ws.get(repo="myrepo"), {"name": "myrepo"})

    async def test_workspace_delete_resolves_workspace_argument(self):
        fake = FakeAsyncMCP()
        ws = AsyncWorkspaceClient(fake)
        result = await ws.delete("myws")
        self.assertEqual(result, {"workspace": "myws", "deleted": True})


class ExportsTest(unittest.TestCase):
    def test_async_names_importable_from_package_root(self):
        import redis_afs
        for name in ["AsyncAFS", "AsyncWorkspaceClient", "AsyncRepoClient",
                     "AsyncCheckpointClient", "AsyncFSClient", "AsyncMountedFS",
                     "AsyncBashRunner", "AsyncMCPHttpClient"]:
            self.assertTrue(hasattr(redis_afs, name), name)


class HttpxGuardTest(unittest.TestCase):
    def test_constructing_client_without_httpx_raises_install_hint(self):
        original = aio._http.httpx
        try:
            aio._http.httpx = None
            with self.assertRaises(AFSError) as ctx:
                AsyncMCPHttpClient(api_key="k")
        finally:
            aio._http.httpx = original
        self.assertIn("httpx", str(ctx.exception))


def _mock_client(handler):
    transport = httpx.MockTransport(handler)
    return AsyncMCPHttpClient(api_key="k", base_url="https://afs.example", transport=transport)


@unittest.skipIf(httpx is None, "httpx not installed")
class AsyncTransportTest(unittest.IsolatedAsyncioTestCase):
    async def test_call_tool_unwraps_structured_content(self):
        seen = {}

        def handler(request):
            seen["url"] = str(request.url)
            seen["body"] = request.content.decode()
            return httpx.Response(200, json={"result": {"structuredContent": {"ok": True}}})

        client = _mock_client(handler)
        try:
            result = await client.call_tool("workspace_list", {"a": None, "b": 1})
        finally:
            await client.aclose()

        self.assertEqual(result, {"ok": True})
        self.assertTrue(seen["url"].endswith("/mcp"))
        self.assertNotIn('"a"', seen["body"])  # strip_none drops a=None

    async def test_tool_error_raises(self):
        def handler(request):
            return httpx.Response(200, json={"result": {"isError": True, "content": [{"text": "boom"}]}})

        client = _mock_client(handler)
        with self.assertRaises(AFSError) as ctx:
            await client.call_tool("x")
        await client.aclose()
        self.assertIn("boom", str(ctx.exception))

    async def test_http_error_carries_status(self):
        def handler(request):
            return httpx.Response(500, text="nope")

        client = _mock_client(handler)
        with self.assertRaises(AFSError) as ctx:
            await client.request("tools/call", {})
        await client.aclose()
        self.assertEqual(ctx.exception.status, 500)

    async def test_timeout_maps_to_afs_error(self):
        def handler(request):
            raise httpx.TimeoutException("slow")

        client = _mock_client(handler)
        with self.assertRaises(AFSError) as ctx:
            await client.request("tools/call", {})
        await client.aclose()
        self.assertIn("timed out", str(ctx.exception))

    async def test_async_context_manager_closes(self):
        def handler(request):
            return httpx.Response(200, json={"result": {"structuredContent": {}}})

        async with _mock_client(handler) as client:
            await client.call_tool("x")
        self.assertTrue(client._client.is_closed)


class AsyncMountedFSTest(unittest.IsolatedAsyncioTestCase):
    async def test_single_workspace_paths_are_workspace_relative(self):
        fake = FakeAsyncMCP()
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="foobar", token="t", client=fake)])
        await fs.write_file("/src/README.md", "hello")
        self.assertEqual(fake.files["/src/README.md"], "hello")
        self.assertEqual(await fs.read_file("/foobar/src/README.md"), "hello")
        self.assertEqual(fs.workspace_names, ["foobar"])

    async def test_multi_workspace_requires_prefix(self):
        fs = AsyncMountedFS([
            _AsyncMountedWorkspace(name="api", token="t", client=FakeAsyncMCP()),
            _AsyncMountedWorkspace(name="web", token="t", client=FakeAsyncMCP()),
        ])
        with self.assertRaises(AFSError):
            await fs.write_file("/README.md", "hello")


class MountTableTest(unittest.TestCase):
    def test_single_workspace_fallback(self):
        from redis_afs._paths import MountTable
        table = MountTable(["only"])
        self.assertEqual(table.resolve("/src/app.py"), ("only", "/src/app.py"))

    def test_exact_prefix_match(self):
        from redis_afs._paths import MountTable
        table = MountTable(["api", "web"])
        self.assertEqual(table.resolve("/api"), ("api", "/"))
        self.assertEqual(table.resolve("/web/index.html"), ("web", "/index.html"))

    def test_multi_workspace_requires_prefix(self):
        from redis_afs._paths import MountTable
        table = MountTable(["api", "web"])
        with self.assertRaises(AFSError) as ctx:
            table.resolve("/README.md")
        self.assertIn("must start with one of", str(ctx.exception))
        self.assertIn("/api", str(ctx.exception))
        self.assertIn("/web", str(ctx.exception))


class FakeAsyncControlPlane:
    def __init__(self):
        self.issued = []
        self.timeout = 30.0
        self.endpoint = "https://afs.example/mcp"

    async def call_tool(self, name, arguments=None):
        arguments = arguments or {}
        if name != "mcp_token_issue":
            raise AssertionError(f"unexpected tool {name}")
        token = f"workspace-token-{arguments['workspace']}"
        self.issued.append({"name": name, "arguments": dict(arguments), "token": token})
        return {
            "token": token,
            "url": "https://afs.example/mcp",
            "workspace": arguments["workspace"],
            "profile": arguments["profile"],
        }


class FakeAsyncMountedClient:
    files_by_token = {}

    def __init__(self, *, api_key, base_url=None, timeout=30.0, headers=None):
        self.api_key = api_key
        self.endpoint = base_url or "https://afs.example/mcp"
        self.timeout = timeout
        self.headers = dict(headers or {})
        self.closed = False

    async def call_tool(self, name, arguments=None):
        arguments = arguments or {}
        files = self.files_by_token.setdefault(self.api_key, {})
        if name == "file_write":
            files[arguments["path"]] = arguments["content"]
            return {"path": arguments["path"], "operation": "write"}
        if name == "file_read":
            return {"path": arguments["path"], "kind": "file", "content": files.get(arguments["path"], "")}
        raise AssertionError(f"unexpected tool {name}")

    async def aclose(self):
        self.closed = True


class AsyncFSMountTest(unittest.IsolatedAsyncioTestCase):
    async def test_mount_issues_token_and_round_trips(self):
        control_plane = FakeAsyncControlPlane()
        FakeAsyncMountedClient.files_by_token = {}

        with patch("redis_afs.aio._resources.AsyncMCPHttpClient", FakeAsyncMountedClient):
            fs = await AsyncFSClient(control_plane).mount(
                workspaces=[{"name": "repo"}], mode="rw", token_name="Mounted FS"
            )
            try:
                await fs.write_file("/repo/README.md", "hello from mounted fs")
                self.assertEqual(await fs.read_file("/repo/README.md"), "hello from mounted fs")
                self.assertEqual(fs.workspace_names, ["repo"])
                self.assertEqual(control_plane.issued[0]["arguments"]["workspace"], "repo")
                self.assertEqual(control_plane.issued[0]["arguments"]["profile"], "workspace-rw")
                self.assertEqual(control_plane.issued[0]["arguments"]["name"], "Mounted FS")
                child = fs._workspaces[0].client
                self.assertFalse(child.closed)
            finally:
                await fs.aclose()
            self.assertTrue(child.closed)

    async def test_mount_issues_distinct_tokens_per_workspace(self):
        control_plane = FakeAsyncControlPlane()
        FakeAsyncMountedClient.files_by_token = {}

        with patch("redis_afs.aio._resources.AsyncMCPHttpClient", FakeAsyncMountedClient):
            fs = await AsyncFSClient(control_plane).mount(
                workspaces=[{"name": "api"}, {"name": "web"}], mode="rw"
            )
            try:
                self.assertEqual(fs.workspace_names, ["api", "web"])
                self.assertEqual(len(control_plane.issued), 2)
                self.assertEqual(control_plane.issued[0]["arguments"]["workspace"], "api")
                self.assertEqual(control_plane.issued[1]["arguments"]["workspace"], "web")
            finally:
                await fs.aclose()

    async def test_mount_issues_all_tokens_concurrently(self):
        control_plane = FakeAsyncControlPlane()
        FakeAsyncMountedClient.files_by_token = {}

        with patch("redis_afs.aio._resources.AsyncMCPHttpClient", FakeAsyncMountedClient):
            fs = await AsyncFSClient(control_plane).mount(
                workspaces=[{"name": "api"}, {"name": "web"}, {"name": "db"}], mode="rw"
            )
            try:
                self.assertEqual(len(control_plane.issued), 3)
                # mounted ordering is deterministic, matching the input refs.
                self.assertEqual(fs.workspace_names, ["api", "web", "db"])
            finally:
                await fs.aclose()

    async def test_mount_requires_at_least_one_workspace(self):
        with self.assertRaises(AFSError):
            await AsyncFSClient(FakeAsyncControlPlane()).mount(workspaces=[])

    async def test_mount_forwards_concurrency(self):
        control_plane = FakeAsyncControlPlane()
        FakeAsyncMountedClient.files_by_token = {}

        with patch("redis_afs.aio._resources.AsyncMCPHttpClient", FakeAsyncMountedClient):
            fs = await AsyncFSClient(control_plane).mount(
                workspaces=[{"name": "repo"}], concurrency=3
            )
            try:
                self.assertEqual(fs._concurrency, 3)
            finally:
                await fs.aclose()


class AsyncSyncTest(unittest.IsolatedAsyncioTestCase):
    async def test_round_trip_materializes_files(self):
        fake = FakeAsyncMCP()
        fake.files["/README.md"] = "hello"
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        root = await fs.sync_from_remote()
        self.assertEqual(Path(root, "repo", "README.md").read_text(), "hello")

    async def test_sync_to_remote_skips_symlinks(self):
        fake = FakeAsyncMCP()
        fake.files["/README.md"] = "hello"
        fake.symlinks["/link.md"] = "README.md"
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        root = await fs.sync_from_remote()
        self.assertTrue(Path(root, "repo", "link.md").is_symlink())
        await fs.sync_to_remote()
        writes = [a["path"] for n, a in fake.calls if n == "file_write"]
        self.assertNotIn("/link.md", writes)


class ConcurrencyFakeMCP(FakeAsyncMCP):
    def __init__(self):
        super().__init__()
        self.in_flight = 0
        self.max_in_flight = 0

    async def call_tool(self, name, arguments=None):
        if name == "file_read":
            self.in_flight += 1
            self.max_in_flight = max(self.max_in_flight, self.in_flight)
            try:
                # Yield repeatedly so other concurrent reads can enter the
                # critical section before we decrement, exposing real overlap.
                for _ in range(5):
                    await asyncio.sleep(0)
                return await super().call_tool(name, arguments)
            finally:
                self.in_flight -= 1
        return await super().call_tool(name, arguments)


class AsyncSyncConcurrencyTest(unittest.IsolatedAsyncioTestCase):
    async def test_sync_from_remote_bounds_and_overlaps_reads(self):
        fake = ConcurrencyFakeMCP()
        for i in range(8):
            fake.files[f"/file{i}.txt"] = f"content-{i}"
        fs = AsyncMountedFS(
            [_AsyncMountedWorkspace(name="repo", token="t", client=fake)],
            concurrency=3,
        )
        self.addAsyncCleanup(fs.aclose)
        await fs.sync_from_remote()
        self.assertLessEqual(fake.max_in_flight, 3)
        self.assertGreater(fake.max_in_flight, 1)


class AsyncBashTest(unittest.IsolatedAsyncioTestCase):
    async def test_exec_runs_command_and_syncs(self):
        fake = FakeAsyncMCP()
        fake.files["/hello.txt"] = "hi"
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        result = await fs.bash().exec("cat /repo/hello.txt")
        self.assertIsInstance(result, BashResult)
        self.assertEqual(result.exit_code, 0)
        self.assertEqual(result.stdout.strip(), "hi")

    async def test_exec_check_raises_on_nonzero(self):
        fake = FakeAsyncMCP()
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        # Without check=True, a non-zero exit is returned, not raised.
        result = await fs.bash().exec("exit 3")
        self.assertIsInstance(result, BashResult)
        self.assertEqual(result.exit_code, 3)
        # With check=True, it raises AFSError carrying the result as payload.
        with self.assertRaises(AFSError) as ctx:
            await fs.bash().exec("exit 3", check=True)
        self.assertIs(ctx.exception.payload.exit_code, 3)

    async def test_exec_env_override(self):
        fake = FakeAsyncMCP()
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        result = await fs.bash().exec("echo $FOO", env={"FOO": "bar"})
        self.assertEqual(result.exit_code, 0)
        self.assertIn("bar", result.stdout)
        # A value of None removes the variable from the environment.
        removed = await fs.bash().exec("echo [$FOO]", env={"FOO": None})
        self.assertEqual(removed.stdout.strip(), "[]")

    async def test_exec_timeout_raises_and_reaps(self):
        import asyncio
        fake = FakeAsyncMCP()
        fs = AsyncMountedFS([_AsyncMountedWorkspace(name="repo", token="t", client=fake)])
        self.addAsyncCleanup(fs.aclose)
        with self.assertRaises(asyncio.TimeoutError):
            await fs.bash().exec("sleep 5", timeout=0.2)


if __name__ == "__main__":
    unittest.main()
