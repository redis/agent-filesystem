from test import TestCase


class TruncateUtimens(TestCase):
    def getname(self):
        return "FS.TRUNCATE and FS.UTIMENS — edge behavior"

    def test(self):
        r = self.redis
        k = self.test_key

        r.execute_command("FS.ECHO", k, "/data.bin", b"abcdef")

        # Shrink.
        assert r.execute_command("FS.TRUNCATE", k, "/data.bin", 3) == b"OK"
        assert r.execute_command("FS.CAT", k, "/data.bin") == b"abc"

        # Extend with zero bytes.
        assert r.execute_command("FS.TRUNCATE", k, "/data.bin", 6) == b"OK"
        assert r.execute_command("FS.CAT", k, "/data.bin") == b"abc\x00\x00\x00"

        # Truncate to empty.
        assert r.execute_command("FS.TRUNCATE", k, "/data.bin", 0) == b"OK"
        assert r.execute_command("FS.CAT", k, "/data.bin") == b""

        # Errors.
        try:
            r.execute_command("FS.TRUNCATE", k, "/data.bin", -1)
            assert False, "Expected negative length to fail"
        except Exception:
            pass

        r.execute_command("FS.MKDIR", k, "/dir")
        try:
            r.execute_command("FS.TRUNCATE", k, "/dir", 1)
            assert False, "Expected truncating directory to fail"
        except Exception:
            pass

        # UTIMENS: set both.
        assert r.execute_command("FS.UTIMENS", k, "/data.bin", 1000, 2000) == b"OK"
        d = dict(zip(
            r.execute_command("FS.STAT", k, "/data.bin")[0::2],
            r.execute_command("FS.STAT", k, "/data.bin")[1::2],
        ))
        assert int(d[b"atime"]) == 1000
        assert int(d[b"mtime"]) == 2000

        # UTIME_OMIT behavior (-1).
        assert r.execute_command("FS.UTIMENS", k, "/data.bin", -1, 3000) == b"OK"
        d2 = dict(zip(
            r.execute_command("FS.STAT", k, "/data.bin")[0::2],
            r.execute_command("FS.STAT", k, "/data.bin")[1::2],
        ))
        assert int(d2[b"atime"]) == 1000
        assert int(d2[b"mtime"]) == 3000
