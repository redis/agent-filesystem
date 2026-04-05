from test import TestCase


class GlobPatterns(TestCase):
    def getname(self):
        return "Glob pattern matching in FS.FIND"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create files with various names.
        for name in ["foo.txt", "bar.txt", "baz.log", "qux.TXT",
                      "abc", "a1c", "a-c", "a!c"]:
            r.execute_command("FS.ECHO", k, f"/{name}", "x")

        # Star wildcard.
        results = r.execute_command("FS.FIND", k, "/", "*.txt")
        names = sorted(results)
        assert b"/foo.txt" in names
        assert b"/bar.txt" in names
        assert b"/baz.log" not in names

        # Question mark wildcard.
        results = r.execute_command("FS.FIND", k, "/", "???.txt")
        names = sorted(results)
        assert b"/foo.txt" in names
        assert b"/bar.txt" in names

        # Character class [abc].
        results = r.execute_command("FS.FIND", k, "/", "[fb]*.txt")
        names = sorted(results)
        assert b"/foo.txt" in names
        assert b"/bar.txt" in names

        # Character range [a-z].
        results = r.execute_command("FS.FIND", k, "/", "a[0-9]c")
        names = [r for r in results]
        assert b"/a1c" in names

        # Negated character class [!x].
        results = r.execute_command("FS.FIND", k, "/", "a[!0-9]c")
        names = sorted(results)
        assert b"/a-c" in names
        assert b"/a!c" in names
        assert b"/a1c" not in names

        # Exact match (no wildcards).
        results = r.execute_command("FS.FIND", k, "/", "abc")
        assert b"/abc" in results
