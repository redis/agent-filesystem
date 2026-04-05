from test import TestCase


class Chmod(TestCase):
    def getname(self):
        return "FS.CHMOD — change file mode"

    def test(self):
        r = self.redis
        k = self.test_key

        r.execute_command("FS.ECHO", k, "/f.txt", "data")

        # Set mode to 0755.
        assert r.execute_command("FS.CHMOD", k, "/f.txt", "0755") == b"OK"
        stat = r.execute_command("FS.STAT", k, "/f.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"mode"] == b"0755", f"Expected 0755, got {d[b'mode']}"

        # Set mode to 0644.
        r.execute_command("FS.CHMOD", k, "/f.txt", "0644")
        stat = r.execute_command("FS.STAT", k, "/f.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"mode"] == b"0644"

        # Chmod on nonexistent path fails.
        try:
            r.execute_command("FS.CHMOD", k, "/nope", "0644")
            assert False, "Expected error on nonexistent path"
        except Exception:
            pass

        # Chmod on directory.
        r.execute_command("FS.MKDIR", k, "/d")
        assert r.execute_command("FS.CHMOD", k, "/d", "0700") == b"OK"
