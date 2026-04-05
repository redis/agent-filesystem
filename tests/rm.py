from test import TestCase
from tests.invariants import assert_tree_consistent


class Rm(TestCase):
    def getname(self):
        return "FS.RM — delete files and directories"

    def test(self):
        r = self.redis
        k = self.test_key

        # Remove a file.
        r.execute_command("FS.ECHO", k, "/file.txt", "data")
        assert r.execute_command("FS.RM", k, "/file.txt") == 1
        assert_tree_consistent(r, k)

        # Removing nonexistent path returns 0.
        assert r.execute_command("FS.RM", k, "/nope") == 0

        # Cannot remove root.
        r.execute_command("FS.ECHO", k, "/keep.txt", "x")
        try:
            r.execute_command("FS.RM", k, "/")
            assert False, "Expected error removing root"
        except Exception:
            pass

        # Non-empty dir without RECURSIVE fails.
        r.execute_command("FS.MKDIR", k, "/dir")
        r.execute_command("FS.ECHO", k, "/dir/child.txt", "x")
        try:
            r.execute_command("FS.RM", k, "/dir")
            assert False, "Expected error on non-empty dir"
        except Exception:
            pass

        # Empty dir can be removed.
        r.execute_command("FS.RM", k, "/dir/child.txt")
        assert r.execute_command("FS.RM", k, "/dir") == 1

        # RECURSIVE removes entire subtree.
        r.execute_command("FS.ECHO", k, "/tree/a/1.txt", "1")
        r.execute_command("FS.ECHO", k, "/tree/a/2.txt", "2")
        r.execute_command("FS.ECHO", k, "/tree/b/3.txt", "3")
        assert r.execute_command("FS.RM", k, "/tree", "RECURSIVE") == 1
        assert r.execute_command("FS.TEST", k, "/tree") == 0
        assert r.execute_command("FS.TEST", k, "/tree/a") == 0
        assert r.execute_command("FS.TEST", k, "/tree/a/1.txt") == 0
        # Key may have auto-deleted; recreate for invariant check.
        r.execute_command("FS.ECHO", k, "/probe.txt", "ok")
        assert_tree_consistent(r, k)
