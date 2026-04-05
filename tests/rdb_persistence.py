from test import TestCase


class RdbPersistence(TestCase):
    def getname(self):
        return "RDB persistence — save/reload preserves data"

    def estimated_runtime(self):
        return 2.0

    def test(self):
        r = self.redis
        k = self.test_key

        # Build a filesystem with files, dirs, symlinks.
        r.execute_command("FS.ECHO", k, "/f.txt", "hello world")
        r.execute_command("FS.MKDIR", k, "/mydir")
        r.execute_command("FS.ECHO", k, "/mydir/a.txt", "aaa")
        r.execute_command("FS.LN", k, "/f.txt", "/link")
        r.execute_command("FS.CHMOD", k, "/f.txt", "0600")
        r.execute_command("FS.CHOWN", k, "/f.txt", 1000, 2000)

        # Snapshot state before reload.
        info_before = r.execute_command("FS.INFO", k)
        content_before = r.execute_command("FS.CAT", k, "/f.txt")
        link_before = r.execute_command("FS.READLINK", k, "/link")

        # Snapshot metadata (type, mode, uid, gid, size — skip timestamps
        # since atime changes on read and ctime/mtime are preserved but
        # might differ in representation).
        stat_before = r.execute_command("FS.STAT", k, "/f.txt")
        d_before = dict(zip(stat_before[0::2], stat_before[1::2]))

        # Force RDB save and reload.
        try:
            r.execute_command("DEBUG", "RELOAD")
        except Exception as e:
            # Some Redis configs may not allow DEBUG.
            if "DEBUG" in str(e).upper():
                print("         (skipped — DEBUG command not enabled)")
                return
            raise

        # Verify everything survived.
        info_after = r.execute_command("FS.INFO", k)
        d_info_before = dict(zip(info_before[0::2], info_before[1::2]))
        d_info_after = dict(zip(info_after[0::2], info_after[1::2]))
        assert d_info_before == d_info_after, \
            f"INFO mismatch: {d_info_before} vs {d_info_after}"

        content_after = r.execute_command("FS.CAT", k, "/f.txt")
        assert content_before == content_after

        stat_after = r.execute_command("FS.STAT", k, "/f.txt")
        d_after = dict(zip(stat_after[0::2], stat_after[1::2]))
        # Compare stable fields only (skip atime which changes on read).
        for field in [b"type", b"mode", b"uid", b"gid", b"size"]:
            assert d_before[field] == d_after[field], \
                f"{field} mismatch: {d_before[field]} vs {d_after[field]}"

        link_after = r.execute_command("FS.READLINK", k, "/link")
        assert link_before == link_after

        # Verify symlink still works (follows link through resolution).
        assert r.execute_command("FS.CAT", k, "/link") == b"hello world"

        # Verify directory listing survived.
        ls = r.execute_command("FS.LS", k, "/mydir")
        assert b"a.txt" in ls

        # Verify grep still works after reload.
        results = r.execute_command("FS.GREP", k, "/", "*hello*")
        paths = [m[0] for m in results]
        assert b"/f.txt" in paths
