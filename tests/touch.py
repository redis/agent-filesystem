from test import TestCase
import time


class Touch(TestCase):
    def getname(self):
        return "FS.TOUCH — create empty files and update timestamps"

    def test(self):
        r = self.redis
        k = self.test_key

        # Touch creates a new empty file.
        assert r.execute_command("FS.TOUCH", k, "/new.txt") == b"OK"
        content = r.execute_command("FS.CAT", k, "/new.txt")
        assert content == b"", f"Expected empty file, got {content!r}"

        # Touch on existing file updates mtime.
        r.execute_command("FS.ECHO", k, "/ts.txt", "data")
        stat1 = r.execute_command("FS.STAT", k, "/ts.txt")
        d1 = dict(zip(stat1[0::2], stat1[1::2]))
        mtime1 = int(d1[b"mtime"])

        time.sleep(1.1)  # ensure clock advances
        r.execute_command("FS.TOUCH", k, "/ts.txt")

        stat2 = r.execute_command("FS.STAT", k, "/ts.txt")
        d2 = dict(zip(stat2[0::2], stat2[1::2]))
        mtime2 = int(d2[b"mtime"])
        assert mtime2 > mtime1, f"mtime did not advance: {mtime1} -> {mtime2}"

        # Content is preserved after touch.
        assert r.execute_command("FS.CAT", k, "/ts.txt") == b"data"

        # Touch auto-creates parent directories.
        r.execute_command("FS.TOUCH", k, "/a/b/c.txt")
        assert r.execute_command("FS.TEST", k, "/a/b/c.txt") == 1

    def estimated_runtime(self):
        return 1.5
