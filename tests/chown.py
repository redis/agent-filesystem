from test import TestCase


class Chown(TestCase):
    def getname(self):
        return "FS.CHOWN — change ownership"

    def test(self):
        r = self.redis
        k = self.test_key

        r.execute_command("FS.ECHO", k, "/f.txt", "data")

        # Set uid only.
        assert r.execute_command("FS.CHOWN", k, "/f.txt", 1000) == b"OK"
        stat = r.execute_command("FS.STAT", k, "/f.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert int(d[b"uid"]) == 1000

        # Set uid and gid.
        assert r.execute_command("FS.CHOWN", k, "/f.txt", 500, 600) == b"OK"
        stat = r.execute_command("FS.STAT", k, "/f.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert int(d[b"uid"]) == 500
        assert int(d[b"gid"]) == 600

        # Chown on nonexistent path fails.
        try:
            r.execute_command("FS.CHOWN", k, "/nope", 0)
            assert False, "Expected error on nonexistent path"
        except Exception:
            pass
