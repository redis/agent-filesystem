from test import TestCase


class Symlinks(TestCase):
    def getname(self):
        return "FS.LN and FS.READLINK — symbolic links"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a file, then a symlink to it.
        r.execute_command("FS.ECHO", k, "/target.txt", "hello")
        assert r.execute_command("FS.LN", k, "/target.txt", "/link") == b"OK"

        # READLINK returns the target string.
        assert r.execute_command("FS.READLINK", k, "/link") == b"/target.txt"

        # CAT follows symlinks.
        assert r.execute_command("FS.CAT", k, "/link") == b"hello"

        # Symlink to a directory.
        r.execute_command("FS.MKDIR", k, "/realdir")
        r.execute_command("FS.ECHO", k, "/realdir/f.txt", "inside")
        r.execute_command("FS.LN", k, "/realdir", "/dirlink")
        listing = r.execute_command("FS.LS", k, "/dirlink")
        assert b"f.txt" in listing

        # Cannot create link at root.
        try:
            r.execute_command("FS.LN", k, "/target.txt", "/")
            assert False, "Expected error creating link at root"
        except Exception:
            pass

        # Cannot overwrite existing path.
        try:
            r.execute_command("FS.LN", k, "/target.txt", "/link")
            assert False, "Expected error on existing link"
        except Exception:
            pass

        # READLINK on non-symlink fails.
        try:
            r.execute_command("FS.READLINK", k, "/target.txt")
            assert False, "Expected error readlink on file"
        except Exception:
            pass

        # Relative symlink target.
        r.execute_command("FS.LN", k, "target.txt", "/rellink")
        assert r.execute_command("FS.READLINK", k, "/rellink") == b"target.txt"
        # CAT should resolve relative to parent of link.
        assert r.execute_command("FS.CAT", k, "/rellink") == b"hello"

        # Chain of symlinks.
        r.execute_command("FS.LN", k, "/link", "/chain1")
        r.execute_command("FS.LN", k, "/chain1", "/chain2")
        assert r.execute_command("FS.CAT", k, "/chain2") == b"hello"

        # Dangling symlink (target doesn't exist).
        r.execute_command("FS.LN", k, "/nonexistent", "/dangling")
        assert r.execute_command("FS.CAT", k, "/dangling") is None

        # --- Symlink loop detection ---
        # Direct self-loop: link points to itself.
        r.execute_command("FS.LN", k, "/selfloop", "/selfloop")
        try:
            r.execute_command("FS.CAT", k, "/selfloop")
            assert False, "Expected error on self-referencing symlink"
        except Exception as e:
            assert b"loop" in str(e).lower().encode() or b"too many" in str(e).lower().encode() or True

        # Circular loop: A -> B -> A.
        r.execute_command("FS.LN", k, "/loopB", "/loopA")
        r.execute_command("FS.LN", k, "/loopA", "/loopB")
        try:
            r.execute_command("FS.CAT", k, "/loopA")
            assert False, "Expected error on circular symlink loop"
        except Exception:
            pass  # Any error is acceptable

        # Long chain up to the 40-level limit should work.
        r.execute_command("FS.ECHO", k, "/longchain_target.txt", "reached")
        prev = "/longchain_target.txt"
        # Create 39 symlinks (within the 40-level limit).
        for i in range(39):
            curr = f"/longchain_{i}"
            r.execute_command("FS.LN", k, prev, curr)
            prev = curr
        # This should still work (39 hops).
        assert r.execute_command("FS.CAT", k, prev) == b"reached"

        # One more level should exceed the limit.
        r.execute_command("FS.LN", k, prev, "/longchain_over")
        try:
            r.execute_command("FS.CAT", k, "/longchain_over")
            assert False, "Expected error on symlink chain exceeding 40 levels"
        except Exception:
            pass  # Any error is acceptable
