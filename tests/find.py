from test import TestCase


class Find(TestCase):
    def getname(self):
        return "FS.FIND — search by name pattern"

    def test(self):
        r = self.redis
        k = self.test_key

        # Build a filesystem.
        r.execute_command("FS.ECHO", k, "/a.txt", "a")
        r.execute_command("FS.ECHO", k, "/b.log", "b")
        r.execute_command("FS.ECHO", k, "/sub/c.txt", "c")
        r.execute_command("FS.ECHO", k, "/sub/d.log", "d")
        r.execute_command("FS.ECHO", k, "/sub/deep/e.txt", "e")

        # Find *.txt from root.
        results = r.execute_command("FS.FIND", k, "/", "*.txt")
        paths = sorted(results)
        assert b"/a.txt" in paths
        assert b"/sub/c.txt" in paths
        assert b"/sub/deep/e.txt" in paths
        assert b"/b.log" not in paths

        # Find *.log from /sub.
        results = r.execute_command("FS.FIND", k, "/sub", "*.log")
        assert results == [b"/sub/d.log"]

        # Pattern with ? wildcard.
        results = r.execute_command("FS.FIND", k, "/", "?.txt")
        paths = sorted(results)
        assert b"/a.txt" in paths
        assert b"/sub/c.txt" in paths

        # TYPE filter — only files.
        r.execute_command("FS.MKDIR", k, "/sub/data")
        results = r.execute_command("FS.FIND", k, "/", "data", "TYPE", "dir")
        assert b"/sub/data" in results

        # No matches.
        results = r.execute_command("FS.FIND", k, "/", "*.xyz")
        assert results == [] or results is None
