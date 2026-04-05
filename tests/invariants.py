def assert_tree_consistent(r, key):
    if r.exists(key) == 0:
        return

    # Verify each listed child exists and each node is referenced by its parent.
    dirs = r.execute_command("FS.FIND", key, "/", "*", "TYPE", "dir") or []
    all_paths = r.execute_command("FS.FIND", key, "/", "*") or []

    # Root should always exist once key exists.
    assert r.execute_command("FS.TEST", key, "/") == 1

    for d in dirs:
        entries = r.execute_command("FS.LS", key, d) or []
        for name in entries:
            if isinstance(d, bytes):
                child = d + (b"" if d == b"/" else b"/") + name
            else:
                child = d + ("" if d == "/" else "/") + name
            assert r.execute_command("FS.TEST", key, child) == 1, f"Missing listed child {child!r}"

    for p in all_paths:
        if p == b"/" or p == "/":
            continue
        if isinstance(p, bytes):
            idx = p.rfind(b"/")
            parent = p[:idx] if idx > 0 else b"/"
            base = p[idx + 1:]
        else:
            idx = p.rfind("/")
            parent = p[:idx] if idx > 0 else "/"
            base = p[idx + 1:]
        listing = r.execute_command("FS.LS", key, parent) or []
        assert base in listing, f"Parent {parent!r} missing child ref {base!r}"
