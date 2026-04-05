from test import TestCase


class Lifecycle(TestCase):
    def getname(self):
        return "Auto-create and auto-delete key lifecycle"

    def test(self):
        r = self.redis
        k = self.test_key

        # Key doesn't exist before first write.
        assert r.exists(k) == 0

        # First write auto-creates key with root.
        r.execute_command("FS.ECHO", k, "/file.txt", "data")
        assert r.exists(k) == 1

        # Deleting everything auto-deletes the key.
        r.execute_command("FS.RM", k, "/file.txt")
        assert r.exists(k) == 0, "Key should auto-delete when empty"

        # Re-create.
        r.execute_command("FS.ECHO", k, "/a.txt", "a")
        r.execute_command("FS.ECHO", k, "/b.txt", "b")
        assert r.exists(k) == 1

        # Deleting one file doesn't remove key.
        r.execute_command("FS.RM", k, "/a.txt")
        assert r.exists(k) == 1

        # Deleting last file removes key.
        r.execute_command("FS.RM", k, "/b.txt")
        assert r.exists(k) == 0

        # Recursive delete of tree removes key.
        r.execute_command("FS.ECHO", k, "/d/a/1.txt", "1")
        r.execute_command("FS.ECHO", k, "/d/b/2.txt", "2")
        assert r.exists(k) == 1
        r.execute_command("FS.RM", k, "/d", "RECURSIVE")
        assert r.exists(k) == 0
