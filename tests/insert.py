from test import TestCase


class Insert(TestCase):
    def getname(self):
        return "FS.INSERT — insert content at line number"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a file with numbered lines.
        r.execute_command("FS.ECHO", k, "/test.txt", "line 1\nline 2\nline 3")

        # Insert after line 1.
        result = r.execute_command("FS.INSERT", k, "/test.txt", 1, "inserted")
        assert result == b"OK", f"Expected OK, got {result}"
        content = r.execute_command("FS.CAT", k, "/test.txt")
        assert content == b"line 1\ninserted\nline 2\nline 3", f"Got {content}"

        # Insert at beginning (line 0 means before line 1).
        r.execute_command("FS.ECHO", k, "/begin.txt", "line 1\nline 2")
        r.execute_command("FS.INSERT", k, "/begin.txt", 0, "header")
        content = r.execute_command("FS.CAT", k, "/begin.txt")
        assert content == b"header\nline 1\nline 2", f"Got {content}"

        # Insert at end.
        r.execute_command("FS.ECHO", k, "/end.txt", "line 1\nline 2")
        r.execute_command("FS.INSERT", k, "/end.txt", 2, "footer")
        content = r.execute_command("FS.CAT", k, "/end.txt")
        assert content == b"line 1\nline 2\nfooter", f"Got {content}"

        # Insert with -1 means append at end.
        r.execute_command("FS.ECHO", k, "/append.txt", "line 1\nline 2")
        r.execute_command("FS.INSERT", k, "/append.txt", -1, "last")
        content = r.execute_command("FS.CAT", k, "/append.txt")
        assert content == b"line 1\nline 2\nlast", f"Got {content}"

        # Insert into empty file.
        r.execute_command("FS.ECHO", k, "/empty.txt", "")
        r.execute_command("FS.INSERT", k, "/empty.txt", 0, "first line")
        content = r.execute_command("FS.CAT", k, "/empty.txt")
        assert content == b"first line", f"Got {content}"

        # Insert multiple lines at once.
        r.execute_command("FS.ECHO", k, "/multi.txt", "A\nB")
        r.execute_command("FS.INSERT", k, "/multi.txt", 1, "X\nY\nZ")
        content = r.execute_command("FS.CAT", k, "/multi.txt")
        assert content == b"A\nX\nY\nZ\nB", f"Got {content}"

        # Insert at line beyond file length - appends at end.
        r.execute_command("FS.ECHO", k, "/short.txt", "only one line")
        r.execute_command("FS.INSERT", k, "/short.txt", 100, "appended")
        content = r.execute_command("FS.CAT", k, "/short.txt")
        assert content == b"only one line\nappended", f"Got {content}"

        # Negative line numbers other than -1 are invalid.
        try:
            r.execute_command("FS.INSERT", k, "/test.txt", -5, "bad")
            assert False, "Expected error for invalid negative line"
        except Exception:
            pass

        # Nonexistent file - should create it.
        r.execute_command("FS.INSERT", k, "/newfile.txt", 0, "created")
        content = r.execute_command("FS.CAT", k, "/newfile.txt")
        assert content == b"created", f"Got {content}"

        # Cannot insert into directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.INSERT", k, "/mydir", 0, "bad")
            assert False, "Expected error inserting into directory"
        except Exception:
            pass

        # File without trailing newline.
        r.execute_command("FS.ECHO", k, "/notail.txt", "no newline")
        r.execute_command("FS.INSERT", k, "/notail.txt", 1, "after")
        content = r.execute_command("FS.CAT", k, "/notail.txt")
        assert content == b"no newline\nafter", f"Got {content}"

