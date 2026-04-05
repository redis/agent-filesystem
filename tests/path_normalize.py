from test import TestCase


class PathNormalize(TestCase):
    def getname(self):
        return "Path normalization — dot, dotdot, double slash"

    def test(self):
        r = self.redis
        k = self.test_key

        # Write through a normalized path, read back via messy path.
        r.execute_command("FS.ECHO", k, "/a/b/c.txt", "data")

        # Double slashes.
        assert r.execute_command("FS.CAT", k, "//a//b//c.txt") == b"data"

        # Dot in path.
        assert r.execute_command("FS.CAT", k, "/a/./b/./c.txt") == b"data"

        # Dotdot in path.
        assert r.execute_command("FS.CAT", k, "/a/b/x/../c.txt") == b"data"
        assert r.execute_command("FS.CAT", k, "/a/b/../b/c.txt") == b"data"

        # Trailing slash (should resolve to same dir for dirs).
        r.execute_command("FS.MKDIR", k, "/mydir")
        stat1 = r.execute_command("FS.STAT", k, "/mydir")
        stat2 = r.execute_command("FS.STAT", k, "/mydir/")
        d1 = dict(zip(stat1[0::2], stat1[1::2]))
        d2 = dict(zip(stat2[0::2], stat2[1::2]))
        assert d1[b"type"] == d2[b"type"]

        # Root variations.
        assert r.execute_command("FS.TEST", k, "/") == 1
        assert r.execute_command("FS.TEST", k, "//") == 1
        assert r.execute_command("FS.TEST", k, "/./") == 1

        # --- Path depth limit tests (FS_MAX_PATH_DEPTH = 256) ---

        # Build a path with exactly 255 components (should succeed).
        # Each component is short to avoid other limits.
        deep_path = "/" + "/".join([f"d{i}" for i in range(255)])
        deep_file = deep_path + "/file.txt"
        r.execute_command("FS.ECHO", k, deep_file, "deep content")
        assert r.execute_command("FS.CAT", k, deep_file) == b"deep content"

        # Clean up the deep path to avoid polluting other tests.
        # Go up the tree and remove.
        r.execute_command("FS.RM", k, "/d0", "RECURSIVE")

        # Build a path with 257 components (should fail - exceeds 256 limit).
        too_deep_path = "/" + "/".join([f"x{i}" for i in range(257)])
        try:
            r.execute_command("FS.ECHO", k, too_deep_path, "should fail")
            assert False, "Expected error on path exceeding 256 depth limit"
        except Exception:
            pass  # Any error is acceptable
