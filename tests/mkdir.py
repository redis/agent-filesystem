from test import TestCase


class Mkdir(TestCase):
    def getname(self):
        return "FS.MKDIR — create directories"

    def test(self):
        r = self.redis
        k = self.test_key

        # Basic mkdir.
        assert r.execute_command("FS.MKDIR", k, "/mydir") == b"OK"
        stat = r.execute_command("FS.STAT", k, "/mydir")
        stat_dict = dict(zip(stat[0::2], stat[1::2]))
        assert stat_dict[b"type"] == b"dir"

        # Mkdir without PARENTS fails if parent missing.
        try:
            r.execute_command("FS.MKDIR", k, "/a/b/c")
            assert False, "Expected error without PARENTS"
        except Exception:
            pass

        # Mkdir with PARENTS creates intermediate dirs.
        assert r.execute_command("FS.MKDIR", k, "/a/b/c", "PARENTS") == b"OK"
        assert r.execute_command("FS.TEST", k, "/a") == 1
        assert r.execute_command("FS.TEST", k, "/a/b") == 1
        assert r.execute_command("FS.TEST", k, "/a/b/c") == 1

        # PARENTS on existing dir is idempotent.
        assert r.execute_command("FS.MKDIR", k, "/a/b/c", "PARENTS") == b"OK"

        # Mkdir on existing path fails (without PARENTS).
        try:
            r.execute_command("FS.MKDIR", k, "/mydir")
            assert False, "Expected error on existing path"
        except Exception:
            pass
