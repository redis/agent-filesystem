from test import TestCase


class Tree(TestCase):
    def getname(self):
        return "FS.TREE — directory tree view"

    def test(self):
        r = self.redis
        k = self.test_key

        # Build a small tree.
        r.execute_command("FS.ECHO", k, "/a.txt", "a")
        r.execute_command("FS.MKDIR", k, "/sub")
        r.execute_command("FS.ECHO", k, "/sub/b.txt", "b")
        r.execute_command("FS.ECHO", k, "/sub/deep/c.txt", "c")

        # Tree from root.
        result = r.execute_command("FS.TREE", k, "/")
        assert result is not None
        assert len(result) > 0

        # Tree from subdirectory.
        result = r.execute_command("FS.TREE", k, "/sub")
        assert result is not None

        # DEPTH limit.
        result_shallow = r.execute_command("FS.TREE", k, "/", "DEPTH", 1)
        result_deep = r.execute_command("FS.TREE", k, "/", "DEPTH", 10)
        # Shallow tree should have fewer elements.
        assert len(result_shallow) <= len(result_deep)

        # Symlinks show up.
        r.execute_command("FS.LN", k, "/a.txt", "/sym")
        result = r.execute_command("FS.TREE", k, "/")
        # Flatten result and look for symlink marker.
        flat = str(result)
        assert "sym" in flat
