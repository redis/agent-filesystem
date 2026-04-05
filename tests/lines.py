from test import TestCase


class Lines(TestCase):
    def getname(self):
        return "FS.LINES — read specific line range"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a file with numbered lines.
        content = "\n".join([f"line {i}" for i in range(1, 11)])  # line 1 to line 10
        r.execute_command("FS.ECHO", k, "/test.txt", content)

        # Read all lines (no range specified).
        result = r.execute_command("FS.LINES", k, "/test.txt")
        assert result == content.encode(), f"Expected full content, got {result}"

        # Read single line (line 1).
        result = r.execute_command("FS.LINES", k, "/test.txt", 1, 1)
        assert result == b"line 1", f"Expected 'line 1', got {result}"

        # Read single line (line 5).
        result = r.execute_command("FS.LINES", k, "/test.txt", 5, 5)
        assert result == b"line 5", f"Expected 'line 5', got {result}"

        # Read range (lines 2-4).
        result = r.execute_command("FS.LINES", k, "/test.txt", 2, 4)
        assert result == b"line 2\nline 3\nline 4", f"Expected lines 2-4, got {result}"

        # Read from line 8 to end (using -1).
        result = r.execute_command("FS.LINES", k, "/test.txt", 8, -1)
        assert result == b"line 8\nline 9\nline 10", f"Expected lines 8-10, got {result}"

        # Read last line only.
        result = r.execute_command("FS.LINES", k, "/test.txt", 10, 10)
        assert result == b"line 10", f"Expected 'line 10', got {result}"

        # Start beyond file length returns empty.
        result = r.execute_command("FS.LINES", k, "/test.txt", 100, 200)
        assert result == b"" or result is None, f"Expected empty, got {result}"

        # End beyond file length is clamped.
        result = r.execute_command("FS.LINES", k, "/test.txt", 9, 100)
        assert result == b"line 9\nline 10", f"Expected lines 9-10, got {result}"

        # Line 0 is invalid (1-indexed).
        try:
            r.execute_command("FS.LINES", k, "/test.txt", 0, 5)
            assert False, "Expected error for line 0"
        except Exception:
            pass

        # Negative start (other than -1 for end) is invalid.
        try:
            r.execute_command("FS.LINES", k, "/test.txt", -5, 10)
            assert False, "Expected error for negative start"
        except Exception:
            pass

        # Nonexistent file returns nil.
        result = r.execute_command("FS.LINES", k, "/nonexistent.txt", 1, 5)
        assert result is None, f"Expected nil for nonexistent file, got {result}"

        # Cannot read lines from a directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.LINES", k, "/mydir", 1, 5)
            assert False, "Expected error reading lines from directory"
        except Exception:
            pass

        # Empty file returns empty.
        r.execute_command("FS.ECHO", k, "/empty.txt", "")
        result = r.execute_command("FS.LINES", k, "/empty.txt", 1, 10)
        assert result == b"" or result is None, f"Expected empty for empty file, got {result}"

        # File with no trailing newline.
        r.execute_command("FS.ECHO", k, "/notail.txt", "one\ntwo\nthree")
        result = r.execute_command("FS.LINES", k, "/notail.txt", 2, 3)
        assert result == b"two\nthree", f"Expected 'two\\nthree', got {result}"

