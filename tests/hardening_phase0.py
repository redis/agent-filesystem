from test import TestCase


class HardeningPhase0(TestCase):
    def getname(self):
        return "Phase 0 hardening regressions"

    def test(self):
        r = self.redis
        k = self.test_key

        r.execute_command("FS.ECHO", k, "/a/file.txt", "payload")

        # 1) Moving a directory into its own subtree must fail.
        try:
            r.execute_command("FS.MV", k, "/a", "/a/sub/new")
            assert False, "Expected error moving directory into its own subtree"
        except Exception:
            pass
        assert r.execute_command("FS.TEST", k, "/a/file.txt") == 1

        # 2) Path depth overflow should fail and must not create truncated aliases.
        deep_path = "/" + "/".join(["d"] * 300)
        truncated_path = "/" + "/".join(["d"] * 256)
        try:
            r.execute_command("FS.TOUCH", k, deep_path)
            assert False, "Expected error on path depth overflow"
        except Exception:
            pass
        assert r.execute_command("FS.TEST", k, truncated_path) == 0

        # 3) CHMOD must reject invalid octal values.
        try:
            r.execute_command("FS.CHMOD", k, "/a/file.txt", "08")
            assert False, "Expected invalid mode to fail"
        except Exception:
            pass
        stat = r.execute_command("FS.STAT", k, "/a/file.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"mode"] == b"0644"

        # 4) CHOWN must reject out-of-range values.
        try:
            r.execute_command("FS.CHOWN", k, "/a/file.txt", -1, 1)
            assert False, "Expected negative uid to fail"
        except Exception:
            pass
        try:
            r.execute_command("FS.CHOWN", k, "/a/file.txt", 1, -1)
            assert False, "Expected negative gid to fail"
        except Exception:
            pass

        # 5) CP must preserve metadata for files and symlinks.
        r.execute_command("FS.CHMOD", k, "/a/file.txt", "0601")
        r.execute_command("FS.CHOWN", k, "/a/file.txt", 12, 34)
        r.execute_command("FS.UTIMENS", k, "/a/file.txt", 1111, 2222)
        r.execute_command("FS.CP", k, "/a/file.txt", "/copy.txt")

        src = dict(zip(
            r.execute_command("FS.STAT", k, "/a/file.txt")[0::2],
            r.execute_command("FS.STAT", k, "/a/file.txt")[1::2],
        ))
        dst = dict(zip(
            r.execute_command("FS.STAT", k, "/copy.txt")[0::2],
            r.execute_command("FS.STAT", k, "/copy.txt")[1::2],
        ))
        for field in [b"type", b"mode", b"uid", b"gid", b"size", b"ctime", b"atime", b"mtime"]:
            assert src[field] == dst[field], f"Field {field} was not preserved"

        r.execute_command("FS.LN", k, "/a/file.txt", "/ln")
        r.execute_command("FS.CHOWN", k, "/ln", 55, 66)
        r.execute_command("FS.UTIMENS", k, "/ln", 3333, 4444)
        r.execute_command("FS.CP", k, "/ln", "/ln-copy")

        lsrc = dict(zip(
            r.execute_command("FS.STAT", k, "/ln")[0::2],
            r.execute_command("FS.STAT", k, "/ln")[1::2],
        ))
        ldst = dict(zip(
            r.execute_command("FS.STAT", k, "/ln-copy")[0::2],
            r.execute_command("FS.STAT", k, "/ln-copy")[1::2],
        ))
        for field in [b"type", b"mode", b"uid", b"gid", b"ctime", b"atime", b"mtime"]:
            assert lsrc[field] == ldst[field], f"Symlink field {field} was not preserved"
        assert r.execute_command("FS.READLINK", k, "/ln") == r.execute_command("FS.READLINK", k, "/ln-copy")
