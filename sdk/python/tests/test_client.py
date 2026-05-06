import unittest
from unittest.mock import patch

from redis_afs.client import AFSError, CheckpointClient, FSClient, MCPHttpClient, MountedFS, _MountedWorkspace, _normalize_mcp_endpoint


class FakeMCP:
    def __init__(self):
        self.files = {}

    def call_tool(self, name, arguments=None):
        arguments = arguments or {}
        if name == "file_write":
            self.files[arguments["path"]] = arguments["content"]
            return {"path": arguments["path"], "operation": "write"}
        if name == "file_read":
            return {
                "path": arguments["path"],
                "kind": "file",
                "content": self.files.get(arguments["path"], ""),
            }
        if name == "file_list":
            path = arguments.get("path", "/")
            entries = []
            for file_path in sorted(self.files):
                if path == "/" and "/" not in file_path.strip("/"):
                    entries.append({"path": file_path, "name": file_path.strip("/"), "kind": "file"})
                elif file_path.startswith(path.rstrip("/") + "/"):
                    remainder = file_path[len(path.rstrip("/")) + 1 :]
                    if "/" not in remainder:
                        entries.append({"path": file_path, "name": remainder, "kind": "file"})
            return {"entries": entries}
        if name == "checkpoint_create":
            return {"workspace": "workspace", "checkpoint": arguments.get("checkpoint") or "auto", "created": True}
        if name == "checkpoint_restore":
            return {"workspace": "workspace", "checkpoint": arguments["checkpoint"], "restored": True}
        raise AssertionError(f"unexpected tool {name}")


class FakeControlPlane:
    def __init__(self):
        self.issued = []
        self.timeout = 30.0
        self.endpoint = "https://afs.example/mcp"

    def call_tool(self, name, arguments=None):
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
            "scope": f"workspace:{arguments['workspace']}",
        }


class FakeMountedMCPHttpClient:
    files_by_token = {}

    def __init__(self, *, api_key, base_url=None, timeout=30.0, headers=None):
        self.api_key = api_key
        self.endpoint = base_url or "https://afs.example/mcp"
        self.timeout = timeout
        self.headers = dict(headers or {})

    def call_tool(self, name, arguments=None):
        arguments = arguments or {}
        files = self.files_by_token.setdefault(self.api_key, {})
        if name == "file_write":
            files[arguments["path"]] = arguments["content"]
            return {"path": arguments["path"], "operation": "write"}
        if name == "file_read":
            return {
                "path": arguments["path"],
                "kind": "file",
                "content": files.get(arguments["path"], ""),
            }
        if name == "file_list":
            path = arguments.get("path", "/")
            entries = []
            for file_path in sorted(files):
                if path == "/" and "/" not in file_path.strip("/"):
                    entries.append({"path": file_path, "name": file_path.strip("/"), "kind": "file"})
                elif file_path.startswith(path.rstrip("/") + "/"):
                    remainder = file_path[len(path.rstrip("/")) + 1 :]
                    if "/" not in remainder:
                        entries.append({"path": file_path, "name": remainder, "kind": "file"})
            return {"entries": entries}
        if name == "checkpoint_create":
            return {"workspace": "workspace", "checkpoint": arguments.get("checkpoint") or "auto", "created": True}
        if name == "checkpoint_restore":
            return {"workspace": "workspace", "checkpoint": arguments["checkpoint"], "restored": True}
        raise AssertionError(f"unexpected tool {name}")


class MountedFSTest(unittest.TestCase):
    def test_single_workspace_paths_are_workspace_relative(self):
        fake = FakeMCP()
        fs = MountedFS([_MountedWorkspace(name="foobar", token="token", client=fake)])

        fs.write_file("/src/README.md", "hello")

        self.assertEqual(fake.files["/src/README.md"], "hello")
        self.assertEqual(fs.read_file("/foobar/src/README.md"), "hello")
        self.assertEqual(fs.workspace_names, ["foobar"])

    def test_multi_workspace_requires_workspace_prefix(self):
        fs = MountedFS(
            [
                _MountedWorkspace(name="api", token="token", client=FakeMCP()),
                _MountedWorkspace(name="web", token="token", client=FakeMCP()),
            ]
        )

        with self.assertRaises(AFSError):
            fs.write_file("/README.md", "hello")

    def test_maps_absolute_workspace_paths_after_materialization(self):
        fake = FakeMCP()
        fake.files["/README.md"] = "hello"
        fs = MountedFS([_MountedWorkspace(name="foobar", token="token", client=fake)])
        self.addCleanup(fs.close)
        root = fs.sync_from_remote()

        mapped = fs.map_absolute_workspace_paths("cat /foobar/README.md")

        self.assertIn(root, mapped)
        self.assertNotEqual(mapped, "cat /foobar/README.md")

    def test_fs_mount_issues_workspace_token_and_reads_and_writes_files(self):
        control_plane = FakeControlPlane()

        with patch("redis_afs.client.MCPHttpClient", FakeMountedMCPHttpClient):
            fs = FSClient(control_plane).mount(workspaces=[{"name": "repo"}], mode="rw", token_name="Mounted FS")
            self.addCleanup(fs.close)

            fs.write_file("/repo/README.md", "hello from mounted fs")

            self.assertEqual(fs.read_file("/repo/README.md"), "hello from mounted fs")
            self.assertEqual(fs.workspace_names, ["repo"])
            self.assertEqual(control_plane.issued[0]["arguments"]["workspace"], "repo")
            self.assertEqual(control_plane.issued[0]["arguments"]["profile"], "workspace-rw")
            self.assertEqual(control_plane.issued[0]["arguments"]["name"], "Mounted FS")


class EndpointTest(unittest.TestCase):
    def test_checkpoint_create_and_restore_round_trip(self):
        checkpoint = CheckpointClient(FakeMCP())

        created = checkpoint.create(workspace="repo", checkpoint="unchanged-head")
        restored = checkpoint.restore(workspace="repo", checkpoint="unchanged-head")

        self.assertTrue(created["created"])
        self.assertEqual(created["checkpoint"], "unchanged-head")
        self.assertTrue(restored["restored"])
        self.assertEqual(restored["checkpoint"], "unchanged-head")

    def test_normalizes_mcp_endpoint(self):
        self.assertEqual(_normalize_mcp_endpoint("https://afs.cloud"), "https://afs.cloud/mcp")
        self.assertEqual(_normalize_mcp_endpoint("https://afs.cloud/mcp"), "https://afs.cloud/mcp")


if __name__ == "__main__":
    unittest.main()
