from test import TestCase
import redis as redispy


class TestCmd(TestCase):
    def getname(self):
        return "FS.TEST — check path existence"

    def test(self):
        r = self.redis
        k = self.test_key

        # Nonexistent key — FS.TEST returns error; treat as "not found".
        try:
            result = r.execute_command("FS.TEST", k, "/nope")
            assert result == 0
        except redispy.ResponseError:
            pass  # key doesn't exist yet, error is acceptable

        # Create a file and test.
        r.execute_command("FS.ECHO", k, "/exists.txt", "data")
        assert r.execute_command("FS.TEST", k, "/exists.txt") == 1

        # Root always exists once key is created.
        assert r.execute_command("FS.TEST", k, "/") == 1

        # Directory.
        r.execute_command("FS.MKDIR", k, "/d")
        assert r.execute_command("FS.TEST", k, "/d") == 1

        # Symlink (existence check, not follow).
        r.execute_command("FS.LN", k, "/exists.txt", "/link")
        assert r.execute_command("FS.TEST", k, "/link") == 1

        # After deletion.
        r.execute_command("FS.RM", k, "/exists.txt")
        assert r.execute_command("FS.TEST", k, "/exists.txt") == 0

        # Path that doesn't exist in existing filesystem.
        assert r.execute_command("FS.TEST", k, "/nonexistent") == 0
