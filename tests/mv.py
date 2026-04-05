from test import TestCase
from tests.invariants import assert_tree_consistent


class Mv(TestCase):
    def getname(self):
        return "FS.MV — move/rename files and directories"

    def test(self):
        r = self.redis
        k = self.test_key

        # Move a file.
        r.execute_command("FS.ECHO", k, "/old.txt", "content")
        assert r.execute_command("FS.MV", k, "/old.txt", "/new.txt") == b"OK"
        assert r.execute_command("FS.CAT", k, "/new.txt") == b"content"
        assert r.execute_command("FS.TEST", k, "/old.txt") == 0

        # Move a directory (all children move too).
        r.execute_command("FS.ECHO", k, "/src/a.txt", "a")
        r.execute_command("FS.ECHO", k, "/src/b.txt", "b")
        r.execute_command("FS.ECHO", k, "/src/sub/c.txt", "c")
        assert r.execute_command("FS.MV", k, "/src", "/dst") == b"OK"
        assert r.execute_command("FS.CAT", k, "/dst/a.txt") == b"a"
        assert r.execute_command("FS.CAT", k, "/dst/sub/c.txt") == b"c"
        assert r.execute_command("FS.TEST", k, "/src") == 0
        assert_tree_consistent(r, k)

        # Cannot move root.
        try:
            r.execute_command("FS.MV", k, "/", "/newroot")
            assert False, "Expected error moving root"
        except Exception:
            pass

        # Error if dst already exists.
        r.execute_command("FS.ECHO", k, "/x.txt", "x")
        r.execute_command("FS.ECHO", k, "/y.txt", "y")
        try:
            r.execute_command("FS.MV", k, "/x.txt", "/y.txt")
            assert False, "Expected error on existing dst"
        except Exception:
            pass

        # Cannot move a directory into its own subtree.
        r.execute_command("FS.ECHO", k, "/self/a.txt", "x")
        try:
            r.execute_command("FS.MV", k, "/self", "/self/sub/new")
            assert False, "Expected error moving directory into its own subtree"
        except Exception:
            pass
        assert r.execute_command("FS.TEST", k, "/self/a.txt") == 1
        assert_tree_consistent(r, k)
