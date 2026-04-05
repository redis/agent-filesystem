from test import TestCase


class Grep(TestCase):
    def getname(self):
        return "FS.GREP — search file contents"

    def test(self):
        r = self.redis
        k = self.test_key

        # Build files with known content.
        r.execute_command("FS.ECHO", k, "/a.txt", "Hello World\nfoo bar\nbaz")
        r.execute_command("FS.ECHO", k, "/b.txt", "nothing here\nHello Again\n")
        r.execute_command("FS.ECHO", k, "/sub/c.txt", "deep hello content\n")

        # Grep for "Hello" (case-sensitive, glob pattern).
        # Each result is a 3-element sub-array: [path, lineno, line].
        results = r.execute_command("FS.GREP", k, "/", "Hello*")
        paths = [m[0] for m in results]
        assert b"/a.txt" in paths, f"Expected /a.txt in {paths}"
        assert b"/b.txt" in paths, f"Expected /b.txt in {paths}"

        # NOCASE grep.
        results = r.execute_command("FS.GREP", k, "/", "*hello*", "NOCASE")
        paths = [m[0] for m in results]
        assert b"/a.txt" in paths
        assert b"/sub/c.txt" in paths

        # Grep with no matches returns empty.
        results = r.execute_command("FS.GREP", k, "/", "zzzzz*")
        assert results == [] or results is None

        # Grep from subdirectory.
        results = r.execute_command("FS.GREP", k, "/sub", "*hello*")
        paths = [m[0] for m in results]
        assert b"/sub/c.txt" in paths
        assert len(paths) == 1

        # Binary file detection.
        r.execute_command("FS.ECHO", k, "/bin.dat",
                          b"start\x00\x00\x00middle hello end")
        results = r.execute_command("FS.GREP", k, "/", "*hello*")
        # Binary file should report "Binary file matches".
        bin_entries = [m for m in results if m[0] == b"/bin.dat"]
        if bin_entries:
            assert b"Binary file" in bin_entries[0][2], \
                f"Expected binary notice, got {bin_entries[0][2]}"
