from test import TestCase


class Stat(TestCase):
    def getname(self):
        return "FS.STAT — inode metadata"

    def test(self):
        r = self.redis
        k = self.test_key

        # Stat a file.
        r.execute_command("FS.ECHO", k, "/f.txt", "hello")
        stat = r.execute_command("FS.STAT", k, "/f.txt")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"type"] == b"file"
        assert int(d[b"size"]) == 5

        # Stat a directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        stat = r.execute_command("FS.STAT", k, "/mydir")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"type"] == b"dir"

        # Stat nonexistent returns nil.
        assert r.execute_command("FS.STAT", k, "/nope") is None

        # Stat does NOT follow symlinks.
        r.execute_command("FS.LN", k, "/f.txt", "/link")
        stat = r.execute_command("FS.STAT", k, "/link")
        d = dict(zip(stat[0::2], stat[1::2]))
        assert d[b"type"] == b"symlink"

        # Mode is returned in octal format.
        assert b"mode" in d
