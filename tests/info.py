from test import TestCase


class Info(TestCase):
    def getname(self):
        return "FS.INFO — filesystem summary"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create some content.
        r.execute_command("FS.ECHO", k, "/f1.txt", "hello")
        r.execute_command("FS.ECHO", k, "/f2.txt", "world!")
        r.execute_command("FS.MKDIR", k, "/mydir")
        r.execute_command("FS.LN", k, "/f1.txt", "/link")

        info = r.execute_command("FS.INFO", k)
        d = dict(zip(info[0::2], info[1::2]))

        assert int(d[b"files"]) == 2, f"Expected 2 files, got {d[b'files']}"
        # root + /mydir = at least 2 dirs.
        assert int(d[b"directories"]) >= 2
        assert int(d[b"symlinks"]) == 1
        assert int(d[b"total_data_bytes"]) == 11  # 5 + 6
        assert int(d[b"total_inodes"]) >= 5  # root + 2 files + 1 dir + 1 link
