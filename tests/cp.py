from test import TestCase
from tests.invariants import assert_tree_consistent


class Cp(TestCase):
    def getname(self):
        return "FS.CP — copy files and directories"

    def test(self):
        r = self.redis
        k = self.test_key

        # Copy a file.
        r.execute_command("FS.ECHO", k, "/src.txt", "data")
        r.execute_command("FS.CHMOD", k, "/src.txt", "0600")
        assert r.execute_command("FS.CP", k, "/src.txt", "/dst.txt") == b"OK"
        assert r.execute_command("FS.CAT", k, "/dst.txt") == b"data"

        # Original unchanged.
        assert r.execute_command("FS.CAT", k, "/src.txt") == b"data"

        # Metadata copied.
        stat = r.execute_command("FS.STAT", k, "/dst.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"mode"] == b"0600"

        # Cannot copy dir without RECURSIVE.
        r.execute_command("FS.MKDIR", k, "/srcdir")
        r.execute_command("FS.ECHO", k, "/srcdir/a.txt", "a")
        try:
            r.execute_command("FS.CP", k, "/srcdir", "/dstdir")
            assert False, "Expected error copying dir without RECURSIVE"
        except Exception:
            pass

        # RECURSIVE copy.
        r.execute_command("FS.ECHO", k, "/srcdir/sub/b.txt", "b")
        assert r.execute_command("FS.CP", k, "/srcdir", "/dstdir", "RECURSIVE") == b"OK"
        assert r.execute_command("FS.CAT", k, "/dstdir/a.txt") == b"a"
        assert r.execute_command("FS.CAT", k, "/dstdir/sub/b.txt") == b"b"
        assert_tree_consistent(r, k)

        # Error if dst already exists.
        try:
            r.execute_command("FS.CP", k, "/src.txt", "/dst.txt")
            assert False, "Expected error on existing dst"
        except Exception:
            pass
