import unittest

from redis_afs.client import MCPHttpClient, AFSError, MountedFS, _MountedWorkspace, _normalize_mcp_endpoint


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


class EndpointTest(unittest.TestCase):
    def test_normalizes_mcp_endpoint(self):
        self.assertEqual(_normalize_mcp_endpoint("https://afs.cloud"), "https://afs.cloud/mcp")
        self.assertEqual(_normalize_mcp_endpoint("https://afs.cloud/mcp"), "https://afs.cloud/mcp")


if __name__ == "__main__":
    unittest.main()
