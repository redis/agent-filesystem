from test import TestCase


class WrongKeyType(TestCase):
    def getname(self):
        return "Wrong key type — FS commands on non-fsObject keys"

    def test(self):
        r = self.redis
        k = self.test_key

        # Create a Redis string (not an fsObject).
        string_key = k + ":string"
        r.set(string_key, "I am a plain Redis string")

        # Create a Redis hash.
        hash_key = k + ":hash"
        r.hset(hash_key, "field", "value")

        # Create a Redis list.
        list_key = k + ":list"
        r.rpush(list_key, "item1", "item2")

        # Create a Redis set.
        set_key = k + ":set"
        r.sadd(set_key, "member1", "member2")

        try:
            # --- Test FS commands on string key ---
            try:
                r.execute_command("FS.CAT", string_key, "/file.txt")
                assert False, "Expected WRONGTYPE error on FS.CAT with string key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.ECHO", string_key, "/file.txt", "data")
                assert False, "Expected WRONGTYPE error on FS.ECHO with string key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.LS", string_key, "/")
                assert False, "Expected WRONGTYPE error on FS.LS with string key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.STAT", string_key, "/")
                assert False, "Expected WRONGTYPE error on FS.STAT with string key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            # --- Test FS commands on hash key ---
            try:
                r.execute_command("FS.CAT", hash_key, "/file.txt")
                assert False, "Expected WRONGTYPE error on FS.CAT with hash key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.MKDIR", hash_key, "/dir")
                assert False, "Expected WRONGTYPE error on FS.MKDIR with hash key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            # --- Test FS commands on list key ---
            try:
                r.execute_command("FS.INFO", list_key)
                assert False, "Expected WRONGTYPE error on FS.INFO with list key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.FIND", list_key, "/", "*.txt")
                assert False, "Expected WRONGTYPE error on FS.FIND with list key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            # --- Test FS commands on set key ---
            try:
                r.execute_command("FS.GREP", set_key, "/", "pattern")
                assert False, "Expected WRONGTYPE error on FS.GREP with set key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

            try:
                r.execute_command("FS.TREE", set_key, "/")
                assert False, "Expected WRONGTYPE error on FS.TREE with set key"
            except Exception as e:
                assert b"WRONGTYPE" in str(e).encode() or "WRONGTYPE" in str(e)

        finally:
            # Clean up the non-fs keys.
            r.delete(string_key, hash_key, list_key, set_key)

