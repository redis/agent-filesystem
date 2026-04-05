#!/usr/bin/env python3
"""
Test framework for Redis-FS module.

Follows the conventions of the Redis vector-sets test suite:
  - Custom TestCase base class with auto-discovery
  - Tests use redis-py and plain assert statements
  - One test class per file in tests/
  - Uses DB 9 to avoid conflicts
  - Colored terminal output

Usage:
    # Start Redis with the module loaded first:
    #   redis-server --loadmodule ./module/fs.so --enable-debug-command yes
    #
    python3 test.py [--port 6379]
"""

import argparse
import importlib
import inspect
import os
import sys
import time
import traceback

import redis


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def colored(text, color):
    """Return ANSI-colored text."""
    colors = {
        "red": "\033[91m",
        "green": "\033[92m",
        "yellow": "\033[93m",
        "cyan": "\033[96m",
        "bold": "\033[1m",
        "reset": "\033[0m",
    }
    return f"{colors.get(color, '')}{text}{colors['reset']}"


def check_redis_empty(r, instance_name):
    """Abort if the target DB already has data."""
    size = r.dbsize()
    if size != 0:
        print(colored(
            f"ERROR: {instance_name} DB 9 is not empty (has {size} keys). "
            "Flush it first or use a different database.", "red"))
        sys.exit(1)


def check_module_loaded(r):
    """Abort if the fs module is not loaded."""
    modules = r.module_list()
    names = [m[b"name"].decode() if isinstance(m[b"name"], bytes) else m["name"]
             for m in modules]
    if "fs" not in names:
        print(colored(
            "ERROR: The 'fs' module is not loaded. "
            "Start Redis with: redis-server --loadmodule ./module/fs.so", "red"))
        sys.exit(1)


# ---------------------------------------------------------------------------
# TestCase base class
# ---------------------------------------------------------------------------

class TestCase:
    """Base class for all Redis-FS tests."""

    def __init__(self, port):
        self.port = port
        self.redis = None
        self.test_key = f"test:{self.__class__.__name__.lower()}"

    def getname(self):
        """Human-readable test name (override in subclass)."""
        return self.__class__.__name__

    def estimated_runtime(self):
        """Expected runtime in seconds (override for slow tests)."""
        return 0.1

    def setup(self):
        """Called before each test — clean slate."""
        self.redis = redis.Redis(host="127.0.0.1", port=self.port, db=9)
        # Delete the test key if it exists from a prior failed run.
        self.redis.delete(self.test_key)

    def teardown(self):
        """Called after each test — clean up."""
        if self.redis:
            self.redis.delete(self.test_key)
            self.redis.close()

    def test(self):
        """Override this method with actual test logic."""
        raise NotImplementedError

    def run(self):
        """Execute the test with setup/teardown and result reporting."""
        name = self.getname()
        try:
            self.setup()
            t0 = time.time()
            self.test()
            elapsed = time.time() - t0
            print(f"  {colored('OK', 'green')}   {name} ({elapsed:.3f}s)")
            return True
        except AssertionError as e:
            elapsed = time.time() - t0
            print(f"  {colored('ERR', 'red')}  {name} ({elapsed:.3f}s)")
            print(f"         {e}")
            traceback.print_exc(file=sys.stdout)
            return False
        except Exception as e:
            elapsed = time.time() - t0 if "t0" in dir() else 0
            print(f"  {colored('ERR', 'red')}  {name} ({elapsed:.3f}s)")
            print(f"         Unexpected error: {e}")
            traceback.print_exc(file=sys.stdout)
            return False
        finally:
            try:
                self.teardown()
            except Exception:
                pass


# ---------------------------------------------------------------------------
# Test discovery
# ---------------------------------------------------------------------------

def find_test_classes(port):
    """Auto-discover TestCase subclasses in tests/*.py."""
    project_dir = os.path.dirname(os.path.abspath(__file__))
    tests_dir = os.path.join(project_dir, "tests")
    if not os.path.isdir(tests_dir):
        print(colored("ERROR: tests/ directory not found.", "red"))
        sys.exit(1)

    # Ensure project root is on sys.path so `from test import TestCase` works.
    if project_dir not in sys.path:
        sys.path.insert(0, project_dir)

    classes = []
    for fname in sorted(os.listdir(tests_dir)):
        if not fname.endswith(".py") or fname.startswith("_"):
            continue
        module_name = fname[:-3]
        spec = importlib.util.spec_from_file_location(
            module_name, os.path.join(tests_dir, fname))
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)

        for _, obj in inspect.getmembers(mod, inspect.isclass):
            if (issubclass(obj, TestCase)
                    and obj is not TestCase
                    and hasattr(obj, "test")):
                classes.append(obj(port))

    # Sort by estimated runtime (fast tests first).
    classes.sort(key=lambda t: t.estimated_runtime())
    return classes


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    # Ensure this module is importable as "test" even when run as __main__.
    # Test files do `from test import TestCase` — without this, Python would
    # re-import test.py as a separate module and issubclass() would fail.
    project_dir = os.path.dirname(os.path.abspath(__file__))
    if project_dir not in sys.path:
        sys.path.insert(0, project_dir)
    sys.modules["test"] = sys.modules[__name__]

    parser = argparse.ArgumentParser(description="Redis-FS test runner")
    parser.add_argument("--port", type=int, default=6379,
                        help="Redis port (default 6379)")
    args = parser.parse_args()

    print("=" * 56)
    print("  Redis-FS Test Suite")
    print(f"  Redis at 127.0.0.1:{args.port}, DB 9")
    print("=" * 56)

    # Preflight checks.
    r = redis.Redis(host="127.0.0.1", port=args.port, db=9)
    try:
        r.ping()
    except redis.ConnectionError:
        print(colored(f"ERROR: Cannot connect to Redis on port {args.port}.", "red"))
        sys.exit(1)
    check_redis_empty(r, "Primary")
    check_module_loaded(r)
    r.close()

    tests = find_test_classes(args.port)
    if not tests:
        print(colored("No tests found in tests/ directory.", "yellow"))
        sys.exit(1)

    print(f"\n  Found {len(tests)} tests.\n")

    passed = 0
    failed = 0
    t_start = time.time()

    for tc in tests:
        ok = tc.run()
        if ok:
            passed += 1
        else:
            failed += 1

    elapsed = time.time() - t_start
    print()
    print(f"  {colored(f'{passed} passed', 'green')}, "
          f"{colored(f'{failed} failed', 'red') if failed else '0 failed'} "
          f"({elapsed:.2f}s)")

    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
