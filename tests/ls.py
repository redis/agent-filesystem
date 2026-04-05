from test import TestCase


class Ls(TestCase):
    def getname(self):
        return "FS.LS — list directory contents"

    def test(self):
        r = self.redis
        k = self.test_key

        # Build a small filesystem.
        r.execute_command("FS.ECHO", k, "/a.txt", "aaa")
        r.execute_command("FS.ECHO", k, "/b.txt", "bbb")
        r.execute_command("FS.MKDIR", k, "/subdir")

        # Default path is root.
        entries = r.execute_command("FS.LS", k)
        names = sorted(entries)
        assert b"a.txt" in names
        assert b"b.txt" in names
        assert b"subdir" in names

        # Explicit root path.
        entries2 = r.execute_command("FS.LS", k, "/")
        assert sorted(entries2) == names

        # List a subdirectory.
        r.execute_command("FS.ECHO", k, "/subdir/x.txt", "x")
        entries3 = r.execute_command("FS.LS", k, "/subdir")
        assert entries3 == [b"x.txt"]

        # LONG format returns nested arrays: each entry is [name, type, mode, size, mtime].
        long_entries = r.execute_command("FS.LS", k, "/subdir", "LONG")
        assert len(long_entries) == 1  # one child
        assert long_entries[0][0] == b"x.txt"
        assert long_entries[0][1] == b"file"

        # Listing a file should fail.
        try:
            r.execute_command("FS.LS", k, "/a.txt")
            assert False, "Expected error listing a file"
        except Exception:
            pass

        # Empty directory.
        r.execute_command("FS.MKDIR", k, "/empty")
        entries4 = r.execute_command("FS.LS", k, "/empty")
        assert entries4 == [] or entries4 is None
