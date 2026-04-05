from test import TestCase


class EchoCat(TestCase):
    def getname(self):
        return "FS.ECHO and FS.CAT — basic file read/write"

    def test(self):
        r = self.redis
        k = self.test_key

        # Write a file and read it back.
        assert r.execute_command("FS.ECHO", k, "/hello.txt", "Hello, world!") == b"OK"
        assert r.execute_command("FS.CAT", k, "/hello.txt") == b"Hello, world!"

        # Overwrite replaces content.
        r.execute_command("FS.ECHO", k, "/hello.txt", "Replaced")
        assert r.execute_command("FS.CAT", k, "/hello.txt") == b"Replaced"

        # Empty file.
        r.execute_command("FS.ECHO", k, "/empty.txt", "")
        assert r.execute_command("FS.CAT", k, "/empty.txt") == b""

        # Binary-safe content.
        data = b"\x00\x01\x02\xff"
        r.execute_command("FS.ECHO", k, "/bin.dat", data)
        assert r.execute_command("FS.CAT", k, "/bin.dat") == data

        # CAT on nonexistent path returns nil.
        assert r.execute_command("FS.CAT", k, "/nope.txt") is None

        # Cannot write to root.
        try:
            r.execute_command("FS.ECHO", k, "/", "bad")
            assert False, "Expected error writing to root"
        except Exception:
            pass

        # Auto-creates parent directories.
        r.execute_command("FS.ECHO", k, "/a/b/c/deep.txt", "deep")
        assert r.execute_command("FS.CAT", k, "/a/b/c/deep.txt") == b"deep"
        # Verify intermediate dirs exist.
        assert r.execute_command("FS.TEST", k, "/a") == 1
        assert r.execute_command("FS.TEST", k, "/a/b") == 1
        assert r.execute_command("FS.TEST", k, "/a/b/c") == 1

        # --- FS.ECHO with APPEND flag ---

        # APPEND to existing file appends content.
        r.execute_command("FS.ECHO", k, "/append.txt", "hello")
        assert r.execute_command("FS.ECHO", k, "/append.txt", " world", "APPEND") == b"OK"
        assert r.execute_command("FS.CAT", k, "/append.txt") == b"hello world"

        # APPEND creates file if it doesn't exist.
        assert r.execute_command("FS.ECHO", k, "/new-append.txt", "created", "APPEND") == b"OK"
        assert r.execute_command("FS.CAT", k, "/new-append.txt") == b"created"

        # APPEND auto-creates parent directories.
        r.execute_command("FS.ECHO", k, "/x/y/z.txt", "data", "APPEND")
        assert r.execute_command("FS.CAT", k, "/x/y/z.txt") == b"data"
        assert r.execute_command("FS.TEST", k, "/x/y") == 1

        # APPEND to a directory should fail.
        r.execute_command("FS.MKDIR", k, "/adir")
        try:
            r.execute_command("FS.ECHO", k, "/adir", "bad", "APPEND")
            assert False, "Expected error appending to directory"
        except Exception:
            pass

        # APPEND flag is case-insensitive.
        r.execute_command("FS.ECHO", k, "/append.txt", "!", "append")
        assert r.execute_command("FS.CAT", k, "/append.txt") == b"hello world!"
