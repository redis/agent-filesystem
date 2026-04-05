from test import TestCase


class DeleteLines(TestCase):
    def getname(self):
        return "FS.DELETELINES — delete line range"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a file with numbered lines.
        content = "line 1\nline 2\nline 3\nline 4\nline 5"
        r.execute_command("FS.ECHO", k, "/test.txt", content)

        # Delete single line (line 3).
        result = r.execute_command("FS.DELETELINES", k, "/test.txt", 3, 3)
        assert result == 1, f"Expected 1 line deleted, got {result}"
        got = r.execute_command("FS.CAT", k, "/test.txt")
        assert got == b"line 1\nline 2\nline 4\nline 5", f"Got {got}"

        # Delete range (lines 2-3, which are now "line 2" and "line 4").
        result = r.execute_command("FS.DELETELINES", k, "/test.txt", 2, 3)
        assert result == 2, f"Expected 2 lines deleted, got {result}"
        got = r.execute_command("FS.CAT", k, "/test.txt")
        assert got == b"line 1\nline 5", f"Got {got}"

        # Delete first line.
        r.execute_command("FS.ECHO", k, "/first.txt", "A\nB\nC")
        r.execute_command("FS.DELETELINES", k, "/first.txt", 1, 1)
        got = r.execute_command("FS.CAT", k, "/first.txt")
        assert got == b"B\nC", f"Got {got}"

        # Delete last line.
        r.execute_command("FS.ECHO", k, "/last.txt", "A\nB\nC")
        r.execute_command("FS.DELETELINES", k, "/last.txt", 3, 3)
        got = r.execute_command("FS.CAT", k, "/last.txt")
        # After deleting last line, trailing newline is preserved from line 2.
        assert got == b"A\nB\n", f"Got {got}"

        # Delete all lines.
        r.execute_command("FS.ECHO", k, "/all.txt", "A\nB\nC")
        r.execute_command("FS.DELETELINES", k, "/all.txt", 1, 3)
        got = r.execute_command("FS.CAT", k, "/all.txt")
        assert got == b"", f"Got {got}"

        # End beyond file length is clamped.
        r.execute_command("FS.ECHO", k, "/clamp.txt", "A\nB\nC")
        result = r.execute_command("FS.DELETELINES", k, "/clamp.txt", 2, 100)
        assert result == 2, f"Expected 2 lines deleted, got {result}"
        got = r.execute_command("FS.CAT", k, "/clamp.txt")
        # Line A ends with newline which is preserved.
        assert got == b"A\n", f"Got {got}"

        # Start beyond file length - nothing deleted.
        r.execute_command("FS.ECHO", k, "/beyond.txt", "A\nB")
        result = r.execute_command("FS.DELETELINES", k, "/beyond.txt", 100, 200)
        assert result == 0, f"Expected 0 lines deleted, got {result}"
        got = r.execute_command("FS.CAT", k, "/beyond.txt")
        assert got == b"A\nB", f"Got {got}"

        # Invalid line numbers.
        try:
            r.execute_command("FS.DELETELINES", k, "/test.txt", 0, 5)
            assert False, "Expected error for line 0"
        except Exception:
            pass

        try:
            r.execute_command("FS.DELETELINES", k, "/test.txt", 3, 1)
            assert False, "Expected error for end < start"
        except Exception:
            pass

        # Nonexistent file.
        result = r.execute_command("FS.DELETELINES", k, "/nonexistent.txt", 1, 5)
        assert result is None or result == 0

        # Cannot delete lines from directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.DELETELINES", k, "/mydir", 1, 5)
            assert False, "Expected error deleting lines from directory"
        except Exception:
            pass

        # File without trailing newline.
        r.execute_command("FS.ECHO", k, "/notail.txt", "A\nB\nC")
        r.execute_command("FS.DELETELINES", k, "/notail.txt", 2, 2)
        got = r.execute_command("FS.CAT", k, "/notail.txt")
        assert got == b"A\nC", f"Got {got}"

        # Single line file.
        r.execute_command("FS.ECHO", k, "/single.txt", "only one")
        r.execute_command("FS.DELETELINES", k, "/single.txt", 1, 1)
        got = r.execute_command("FS.CAT", k, "/single.txt")
        assert got == b"", f"Got {got}"

