from test import TestCase


class ErrorHandling(TestCase):
    def getname(self):
        return "Error handling — invalid arguments and edge cases"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a basic filesystem for testing.
        r.execute_command("FS.ECHO", k, "/file.txt", "content")
        r.execute_command("FS.MKDIR", k, "/dir")

        # --- Wrong argument counts ---

        # FS.ECHO requires at least key, path, content.
        try:
            r.execute_command("FS.ECHO", k)
            assert False, "Expected error: FS.ECHO missing path and content"
        except Exception:
            pass

        try:
            r.execute_command("FS.ECHO", k, "/path")
            assert False, "Expected error: FS.ECHO missing content"
        except Exception:
            pass

        # FS.CAT requires key and path.
        try:
            r.execute_command("FS.CAT")
            assert False, "Expected error: FS.CAT missing key"
        except Exception:
            pass

        # FS.MKDIR requires key and path.
        try:
            r.execute_command("FS.MKDIR", k)
            assert False, "Expected error: FS.MKDIR missing path"
        except Exception:
            pass

        # FS.LN requires key, target, and link path.
        try:
            r.execute_command("FS.LN", k, "/target")
            assert False, "Expected error: FS.LN missing link path"
        except Exception:
            pass

        # FS.CP requires key, src, and dst.
        try:
            r.execute_command("FS.CP", k, "/src")
            assert False, "Expected error: FS.CP missing dst"
        except Exception:
            pass

        # FS.MV requires key, src, and dst.
        try:
            r.execute_command("FS.MV", k, "/src")
            assert False, "Expected error: FS.MV missing dst"
        except Exception:
            pass

        # --- Invalid CHMOD values ---

        # Non-numeric mode.
        try:
            r.execute_command("FS.CHMOD", k, "/file.txt", "invalid")
            assert False, "Expected error: CHMOD with non-numeric mode"
        except Exception:
            pass

        # Negative mode.
        try:
            r.execute_command("FS.CHMOD", k, "/file.txt", "-1")
            assert False, "Expected error: CHMOD with negative mode"
        except Exception:
            pass

        # Mode > 07777.
        try:
            r.execute_command("FS.CHMOD", k, "/file.txt", "99999")
            assert False, "Expected error: CHMOD with mode > 07777"
        except Exception:
            pass

        # CHMOD on nonexistent path.
        try:
            r.execute_command("FS.CHMOD", k, "/nonexistent", "0644")
            assert False, "Expected error: CHMOD on nonexistent path"
        except Exception:
            pass

        # --- Invalid CHOWN values ---

        # Non-numeric uid.
        try:
            r.execute_command("FS.CHOWN", k, "/file.txt", "baduid", "0")
            assert False, "Expected error: CHOWN with non-numeric uid"
        except Exception:
            pass

        # Non-numeric gid.
        try:
            r.execute_command("FS.CHOWN", k, "/file.txt", "0", "badgid")
            assert False, "Expected error: CHOWN with non-numeric gid"
        except Exception:
            pass

        # CHOWN on nonexistent path.
        try:
            r.execute_command("FS.CHOWN", k, "/nonexistent", "1000", "1000")
            assert False, "Expected error: CHOWN on nonexistent path"
        except Exception:
            pass

        # --- Invalid flag/option values ---

        # Unknown flag for FS.RM.
        try:
            r.execute_command("FS.RM", k, "/file.txt", "BADOPTION")
            assert False, "Expected error: FS.RM with unknown option"
        except Exception:
            pass

        # Unknown flag for FS.MKDIR.
        try:
            r.execute_command("FS.MKDIR", k, "/newdir", "BADOPTION")
            assert False, "Expected error: FS.MKDIR with unknown option"
        except Exception:
            pass

        # Unknown TYPE value for FS.FIND.
        try:
            r.execute_command("FS.FIND", k, "/", "*", "TYPE", "badtype")
            assert False, "Expected error: FS.FIND with invalid TYPE"
        except Exception:
            pass

        # --- Operations on wrong inode type ---

        # FS.LS on a file (not a directory).
        try:
            r.execute_command("FS.LS", k, "/file.txt")
            assert False, "Expected error: FS.LS on a file"
        except Exception:
            pass

        # FS.READLINK on a file (not a symlink).
        try:
            r.execute_command("FS.READLINK", k, "/file.txt")
            assert False, "Expected error: FS.READLINK on a file"
        except Exception:
            pass

        # FS.READLINK on a directory.
        try:
            r.execute_command("FS.READLINK", k, "/dir")
            assert False, "Expected error: FS.READLINK on a directory"
        except Exception:
            pass

