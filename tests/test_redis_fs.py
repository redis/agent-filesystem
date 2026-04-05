"""Integration tests for Redis-FS Python library.

Requires Redis server with fs.so loaded on port 6399:
    redis-server --port 6399 --loadmodule ./fs.so
"""

import pytest
import redis

from redis_fs import RedisFS


@pytest.fixture
def redis_client():
    """Create Redis client for testing."""
    r = redis.Redis(host="localhost", port=6399, db=0)
    yield r
    # Cleanup: delete test keys
    for key in r.scan_iter("test-*"):
        r.delete(key)


@pytest.fixture
def fs(redis_client):
    """Create RedisFS instance for testing."""
    # Ensure clean state
    redis_client.delete("test-vol")
    return RedisFS(redis_client, "test-vol")


class TestBasicIO:
    """Test basic read/write operations."""

    def test_write_and_read(self, fs):
        """Test writing and reading a file."""
        content = "Hello, World!"
        fs.write("/test.txt", content)
        assert fs.read("/test.txt") == content

    def test_read_nonexistent(self, fs):
        """Test reading a file that doesn't exist."""
        assert fs.read("/does-not-exist.txt") is None

    def test_append(self, fs):
        """Test appending to a file."""
        fs.write("/log.txt", "Line 1\n")
        fs.append("/log.txt", "Line 2\n")
        content = fs.read("/log.txt")
        assert content == "Line 1\nLine 2\n"

    def test_write_creates_parents(self, fs):
        """Test that write creates parent directories."""
        fs.write("/a/b/c/file.txt", "nested")
        assert fs.read("/a/b/c/file.txt") == "nested"
        assert fs.exists("/a/b/c")
        assert fs.exists("/a/b")
        assert fs.exists("/a")


class TestLineOperations:
    """Test line-based operations."""

    def test_lines(self, fs):
        """Test reading specific lines."""
        content = "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
        fs.write("/multiline.txt", content)
        result = fs.lines("/multiline.txt", 2, 4)
        assert "Line 2" in result
        assert "Line 3" in result
        assert "Line 4" in result
        assert "Line 1" not in result
        assert "Line 5" not in result

    def test_head(self, fs):
        """Test reading first N lines."""
        lines = "\n".join(f"Line {i}" for i in range(1, 21)) + "\n"
        fs.write("/many-lines.txt", lines)
        result = fs.head("/many-lines.txt", 3)
        assert "Line 1" in result
        assert "Line 2" in result
        assert "Line 3" in result
        assert "Line 4" not in result

    def test_tail(self, fs):
        """Test reading last N lines."""
        lines = "\n".join(f"Line {i}" for i in range(1, 21)) + "\n"
        fs.write("/many-lines.txt", lines)
        result = fs.tail("/many-lines.txt", 3)
        assert "Line 20" in result
        assert "Line 19" in result
        assert "Line 18" in result
        assert "Line 17" not in result

    def test_insert(self, fs):
        """Test inserting content after a line."""
        fs.write("/insert.txt", "Line 1\nLine 2\nLine 3\n")
        fs.insert("/insert.txt", 1, "Inserted\n")
        content = fs.read("/insert.txt")
        lines = content.split("\n")
        assert lines[1] == "Inserted"

    def test_delete_lines(self, fs):
        """Test deleting lines."""
        fs.write("/delete.txt", "Line 1\nLine 2\nLine 3\nLine 4\n")
        count = fs.delete_lines("/delete.txt", 2, 3)
        assert count == 2
        content = fs.read("/delete.txt")
        assert "Line 2" not in content
        assert "Line 3" not in content
        assert "Line 1" in content
        assert "Line 4" in content


class TestEditing:
    """Test editing operations."""

    def test_replace_first(self, fs):
        """Test replacing first occurrence."""
        fs.write("/replace.txt", "foo bar foo baz foo")
        count = fs.replace("/replace.txt", "foo", "XXX", all=False)
        assert count == 1
        content = fs.read("/replace.txt")
        assert content.startswith("XXX")
        assert "foo" in content  # Still has other occurrences

    def test_replace_all(self, fs):
        """Test replacing all occurrences."""
        fs.write("/replace.txt", "foo bar foo baz foo")
        count = fs.replace("/replace.txt", "foo", "XXX", all=True)
        assert count == 3
        content = fs.read("/replace.txt")
        assert "foo" not in content
        assert content.count("XXX") == 3


class TestNavigation:
    """Test navigation operations."""

    def test_ls(self, fs):
        """Test listing directory contents."""
        fs.write("/dir/file1.txt", "1")
        fs.write("/dir/file2.txt", "2")
        fs.mkdir("/dir/subdir")
        entries = fs.ls("/dir")
        assert "file1.txt" in entries
        assert "file2.txt" in entries
        assert "subdir" in entries

    def test_exists(self, fs):
        """Test existence check."""
        fs.write("/exists.txt", "content")
        assert fs.exists("/exists.txt") is True
        assert fs.exists("/not-exists.txt") is False

    def test_find(self, fs):
        """Test finding files by pattern."""
        fs.write("/docs/readme.md", "readme")
        fs.write("/docs/guide.md", "guide")
        fs.write("/docs/config.json", "config")
        results = fs.find("/docs", "*.md")
        assert len(results) == 2
        assert any("readme.md" in r for r in results)
        assert any("guide.md" in r for r in results)

    def test_stat(self, fs):
        """Test getting file metadata."""
        fs.write("/stat-test.txt", "content")
        info = fs.stat("/stat-test.txt")
        assert info is not None
        assert "type" in info or b"type" in info


class TestOrganization:
    """Test organization operations."""

    def test_mkdir(self, fs):
        """Test creating directories."""
        fs.mkdir("/newdir")
        assert fs.exists("/newdir")

    def test_mkdir_parents(self, fs):
        """Test creating directories with parents."""
        fs.mkdir("/a/b/c/d", parents=True)
        assert fs.exists("/a/b/c/d")
        assert fs.exists("/a/b/c")
        assert fs.exists("/a/b")
        assert fs.exists("/a")

    def test_rm(self, fs):
        """Test removing files."""
        fs.write("/to-delete.txt", "content")
        assert fs.exists("/to-delete.txt")
        fs.rm("/to-delete.txt")
        assert fs.exists("/to-delete.txt") is False

    def test_rm_recursive(self, fs):
        """Test removing directories recursively."""
        fs.write("/dir-to-delete/file1.txt", "1")
        fs.write("/dir-to-delete/subdir/file2.txt", "2")
        fs.rm("/dir-to-delete", recursive=True)
        assert fs.exists("/dir-to-delete") is False

    def test_cp(self, fs):
        """Test copying files."""
        fs.write("/original.txt", "original content")
        fs.cp("/original.txt", "/copy.txt")
        assert fs.read("/copy.txt") == "original content"
        assert fs.read("/original.txt") == "original content"

    def test_mv(self, fs):
        """Test moving files."""
        fs.write("/source.txt", "content")
        fs.mv("/source.txt", "/dest.txt")
        assert fs.exists("/source.txt") is False
        assert fs.read("/dest.txt") == "content"

    def test_ln(self, fs):
        """Test creating symlinks."""
        fs.write("/target.txt", "target content")
        fs.ln("/target.txt", "/link.txt")
        assert fs.read("/link.txt") == "target content"
        assert fs.readlink("/link.txt") == "/target.txt"


class TestSearch:
    """Test search operations."""

    def test_grep(self, fs):
        """Test searching file contents."""
        fs.write("/search/file1.txt", "foo bar baz\nqux quux")
        fs.write("/search/file2.txt", "hello world\nfoo again")
        results = fs.grep("/search", "*foo*")
        assert len(results) > 0


class TestStats:
    """Test stats operations."""

    def test_wc(self, fs):
        """Test word count."""
        fs.write("/wc-test.txt", "one two three\nfour five\n")
        stats = fs.wc("/wc-test.txt")
        assert stats is not None
        # Check for expected keys
        assert "lines" in stats or b"lines" in stats

    def test_info(self, fs):
        """Test filesystem info."""
        fs.write("/info-test.txt", "content")
        info = fs.info()
        assert info is not None
        # Should have file_count or similar
        assert len(info) > 0

