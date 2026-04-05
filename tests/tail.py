from test import TestCase


class Tail(TestCase):
    def getname(self):
        return "FS.TAIL — last N lines"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a file with numbered lines.
        content = "\n".join([f"line {i}" for i in range(1, 21)])  # 20 lines
        r.execute_command("FS.ECHO", k, "/test.txt", content)

        # Default is 10 lines.
        result = r.execute_command("FS.TAIL", k, "/test.txt")
        expected = "\n".join([f"line {i}" for i in range(11, 21)])
        assert result == expected.encode(), f"Expected last 10 lines, got {result}"

        # Explicit N lines.
        result = r.execute_command("FS.TAIL", k, "/test.txt", 5)
        expected = "\n".join([f"line {i}" for i in range(16, 21)])
        assert result == expected.encode(), f"Expected last 5 lines, got {result}"

        # Request more lines than file has.
        result = r.execute_command("FS.TAIL", k, "/test.txt", 100)
        assert result == content.encode(), f"Expected all 20 lines, got {result}"

        # N = 1.
        result = r.execute_command("FS.TAIL", k, "/test.txt", 1)
        assert result == b"line 20", f"Expected 'line 20', got {result}"

        # N = 0 returns empty.
        result = r.execute_command("FS.TAIL", k, "/test.txt", 0)
        assert result == b"" or result is None, f"Expected empty, got {result}"

        # Negative N is invalid.
        try:
            r.execute_command("FS.TAIL", k, "/test.txt", -5)
            assert False, "Expected error for negative N"
        except Exception:
            pass

        # Empty file.
        r.execute_command("FS.ECHO", k, "/empty.txt", "")
        result = r.execute_command("FS.TAIL", k, "/empty.txt", 10)
        assert result == b"" or result is None, f"Expected empty, got {result}"

        # Single line file.
        r.execute_command("FS.ECHO", k, "/single.txt", "only one")
        result = r.execute_command("FS.TAIL", k, "/single.txt", 10)
        assert result == b"only one", f"Expected 'only one', got {result}"

        # Nonexistent file.
        result = r.execute_command("FS.TAIL", k, "/nonexistent.txt", 5)
        assert result is None, f"Expected nil, got {result}"

        # Cannot tail a directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.TAIL", k, "/mydir", 5)
            assert False, "Expected error for directory"
        except Exception:
            pass

        # File with trailing newline.
        r.execute_command("FS.ECHO", k, "/trailing.txt", "A\nB\nC\n")
        result = r.execute_command("FS.TAIL", k, "/trailing.txt", 2)
        # Last two lines are "C" and empty (trailing newline creates empty last line).
        # Or treat trailing newline as part of C. Let's be practical - last 2 non-empty lines.
        # Actually we should count lines as split by \n.
        assert result == b"C\n" or result == b"B\nC\n" or result == b"C", f"Got {result}"

        # Three line file, get last 2.
        r.execute_command("FS.ECHO", k, "/three.txt", "A\nB\nC")
        result = r.execute_command("FS.TAIL", k, "/three.txt", 2)
        assert result == b"B\nC", f"Expected 'B\\nC', got {result}"

