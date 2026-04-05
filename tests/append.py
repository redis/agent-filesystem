from test import TestCase


class Append(TestCase):
    def getname(self):
        return "FS.APPEND — append to files"

    def test(self):
        r = self.redis
        k = self.test_key

        # Append creates the file if it doesn't exist.
        size = r.execute_command("FS.APPEND", k, "/log.txt", "line1\n")
        assert size == 6, f"Expected size 6, got {size}"

        # Subsequent appends grow the file.
        size = r.execute_command("FS.APPEND", k, "/log.txt", "line2\n")
        assert size == 12, f"Expected size 12, got {size}"

        content = r.execute_command("FS.CAT", k, "/log.txt")
        assert content == b"line1\nline2\n"

        # Append auto-creates parents.
        r.execute_command("FS.APPEND", k, "/a/b/c.txt", "data")
        assert r.execute_command("FS.CAT", k, "/a/b/c.txt") == b"data"

        # Append to a directory should fail.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.APPEND", k, "/mydir", "bad")
            assert False, "Expected error appending to directory"
        except Exception:
            pass

        # --- Verify FS.ECHO ... APPEND produces identical results ---

        # Create via ECHO APPEND, same as FS.APPEND creating a new file.
        r.execute_command("FS.ECHO", k, "/echo-log.txt", "line1\n", "APPEND")
        assert r.execute_command("FS.CAT", k, "/echo-log.txt") == b"line1\n"

        # Append via ECHO APPEND, same as FS.APPEND appending.
        r.execute_command("FS.ECHO", k, "/echo-log.txt", "line2\n", "APPEND")
        assert r.execute_command("FS.CAT", k, "/echo-log.txt") == b"line1\nline2\n"

        # ECHO APPEND auto-creates parents, same as FS.APPEND.
        r.execute_command("FS.ECHO", k, "/d/e/f.txt", "data", "APPEND")
        assert r.execute_command("FS.CAT", k, "/d/e/f.txt") == b"data"
