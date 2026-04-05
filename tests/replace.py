from test import TestCase


class Replace(TestCase):
    def getname(self):
        return "FS.REPLACE — exact string replacement"

    def test(self):
        r = self.redis
        k = self.test_key

        # Basic replacement.
        r.execute_command("FS.ECHO", k, "/test.txt", "hello world")
        result = r.execute_command("FS.REPLACE", k, "/test.txt", "world", "universe")
        assert result == 1, f"Expected 1 replacement, got {result}"
        assert r.execute_command("FS.CAT", k, "/test.txt") == b"hello universe"

        # Multiple occurrences - replaces first by default.
        r.execute_command("FS.ECHO", k, "/multi.txt", "foo bar foo baz foo")
        result = r.execute_command("FS.REPLACE", k, "/multi.txt", "foo", "XXX")
        assert result == 1, f"Expected 1 replacement, got {result}"
        assert r.execute_command("FS.CAT", k, "/multi.txt") == b"XXX bar foo baz foo"

        # ALL flag replaces all occurrences.
        r.execute_command("FS.ECHO", k, "/all.txt", "foo bar foo baz foo")
        result = r.execute_command("FS.REPLACE", k, "/all.txt", "foo", "XXX", "ALL")
        assert result == 3, f"Expected 3 replacements, got {result}"
        assert r.execute_command("FS.CAT", k, "/all.txt") == b"XXX bar XXX baz XXX"

        # No match returns 0.
        r.execute_command("FS.ECHO", k, "/nomatch.txt", "hello world")
        result = r.execute_command("FS.REPLACE", k, "/nomatch.txt", "xyz", "abc")
        assert result == 0, f"Expected 0 replacements, got {result}"
        assert r.execute_command("FS.CAT", k, "/nomatch.txt") == b"hello world"

        # Replace with empty string (deletion).
        r.execute_command("FS.ECHO", k, "/delete.txt", "hello world")
        result = r.execute_command("FS.REPLACE", k, "/delete.txt", " world", "")
        assert result == 1
        assert r.execute_command("FS.CAT", k, "/delete.txt") == b"hello"

        # Replace empty string with content (insert at beginning - every position).
        # This might be an edge case - let's say empty old_str is an error.
        try:
            r.execute_command("FS.REPLACE", k, "/test.txt", "", "prefix")
            assert False, "Expected error for empty search string"
        except Exception:
            pass

        # LINE constraint - only replace within line range.
        content = "line 1 foo\nline 2 foo\nline 3 foo\nline 4 foo"
        r.execute_command("FS.ECHO", k, "/lines.txt", content)
        result = r.execute_command("FS.REPLACE", k, "/lines.txt", "foo", "BAR", "LINE", 2, 3)
        assert result == 1, f"Expected 1 replacement, got {result}"
        expected = b"line 1 foo\nline 2 BAR\nline 3 foo\nline 4 foo"
        assert r.execute_command("FS.CAT", k, "/lines.txt") == expected

        # LINE constraint with ALL.
        r.execute_command("FS.ECHO", k, "/lines2.txt", content)
        result = r.execute_command("FS.REPLACE", k, "/lines2.txt", "foo", "BAR", "LINE", 2, 3, "ALL")
        assert result == 2, f"Expected 2 replacements, got {result}"
        expected = b"line 1 foo\nline 2 BAR\nline 3 BAR\nline 4 foo"
        assert r.execute_command("FS.CAT", k, "/lines2.txt") == expected

        # Replacement that spans newlines.
        r.execute_command("FS.ECHO", k, "/span.txt", "hello\nworld")
        result = r.execute_command("FS.REPLACE", k, "/span.txt", "hello\nworld", "goodbye")
        assert result == 1
        assert r.execute_command("FS.CAT", k, "/span.txt") == b"goodbye"

        # Replacement that increases file size.
        r.execute_command("FS.ECHO", k, "/grow.txt", "a")
        result = r.execute_command("FS.REPLACE", k, "/grow.txt", "a", "ABCDEFGHIJ")
        assert result == 1
        assert r.execute_command("FS.CAT", k, "/grow.txt") == b"ABCDEFGHIJ"

        # Nonexistent file.
        result = r.execute_command("FS.REPLACE", k, "/nonexistent.txt", "a", "b")
        assert result is None or result == 0

        # Cannot replace in directory.
        r.execute_command("FS.MKDIR", k, "/mydir")
        try:
            r.execute_command("FS.REPLACE", k, "/mydir", "a", "b")
            assert False, "Expected error replacing in directory"
        except Exception:
            pass

        # Case sensitivity (default is case-sensitive).
        r.execute_command("FS.ECHO", k, "/case.txt", "Hello HELLO hello")
        result = r.execute_command("FS.REPLACE", k, "/case.txt", "hello", "X", "ALL")
        assert result == 1  # Only lowercase 'hello' matches
        assert r.execute_command("FS.CAT", k, "/case.txt") == b"Hello HELLO X"

