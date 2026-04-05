from test import TestCase


class Wc(TestCase):
    def getname(self):
        return "FS.WC — line/word/character counts"

    def test(self):
        r = self.redis
        k = self.test_key

        # Simple file.
        r.execute_command("FS.ECHO", k, "/test.txt", "hello world\nfoo bar baz\n")
        result = r.execute_command("FS.WC", k, "/test.txt")
        # Result should be a dict-like array: [lines, N, words, N, chars, N]
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"lines"] == 2, f"Expected 2 lines, got {d}"
        assert d[b"words"] == 5, f"Expected 5 words, got {d}"
        assert d[b"chars"] == 24, f"Expected 24 chars, got {d}"

        # Empty file.
        r.execute_command("FS.ECHO", k, "/empty.txt", "")
        result = r.execute_command("FS.WC", k, "/empty.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"lines"] == 0, f"Expected 0 lines, got {d}"
        assert d[b"words"] == 0, f"Expected 0 words, got {d}"
        assert d[b"chars"] == 0, f"Expected 0 chars, got {d}"

        # Single line no newline.
        r.execute_command("FS.ECHO", k, "/single.txt", "hello world")
        result = r.execute_command("FS.WC", k, "/single.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"lines"] == 1, f"Expected 1 line, got {d}"
        assert d[b"words"] == 2, f"Expected 2 words, got {d}"
        assert d[b"chars"] == 11, f"Expected 11 chars, got {d}"

        # Multiple spaces between words.
        r.execute_command("FS.ECHO", k, "/spaces.txt", "a   b   c")
        result = r.execute_command("FS.WC", k, "/spaces.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"words"] == 3, f"Expected 3 words, got {d}"

        # Tabs and mixed whitespace.
        r.execute_command("FS.ECHO", k, "/tabs.txt", "a\tb\tc")
        result = r.execute_command("FS.WC", k, "/tabs.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"words"] == 3, f"Expected 3 words, got {d}"

        # Only whitespace (3 spaces, newline, tab, newline, 2 spaces = 3 lines).
        r.execute_command("FS.ECHO", k, "/whitespace.txt", "   \n\t\n  ")
        result = r.execute_command("FS.WC", k, "/whitespace.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"words"] == 0, f"Expected 0 words, got {d}"
        assert d[b"lines"] == 3, f"Expected 3 lines, got {d}"
        assert d[b"chars"] == 8, f"Expected 8 chars, got {d}"

        # Nonexistent file.
        result = r.execute_command("FS.WC", k, "/nonexistent.txt")
        assert result is None, f"Expected nil, got {result}"

        # Cannot wc a directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.WC", k, "/mydir")
            assert False, "Expected error for directory"
        except Exception:
            pass

        # File with many lines.
        content = "\n".join([f"line {i}" for i in range(100)])
        r.execute_command("FS.ECHO", k, "/many.txt", content)
        result = r.execute_command("FS.WC", k, "/many.txt")
        d = dict(zip(result[0::2], result[1::2]))
        assert d[b"lines"] == 100, f"Expected 100 lines, got {d}"
        assert d[b"words"] == 200, f"Expected 200 words (line N), got {d}"

