package mcptools

import "testing"

func TestRequiredString(t *testing.T) {
	got, err := RequiredString(map[string]any{"k": "  hi  "}, "k")
	if err != nil {
		t.Fatalf("RequiredString returned err: %v", err)
	}
	if got != "hi" {
		t.Fatalf("RequiredString = %q, want %q", got, "hi")
	}

	if _, err := RequiredString(map[string]any{}, "k"); err == nil {
		t.Fatal("RequiredString missing-key returned no error")
	}
	if _, err := RequiredString(map[string]any{"k": "   "}, "k"); err == nil {
		t.Fatal("RequiredString blank returned no error")
	}
	if _, err := RequiredString(map[string]any{"k": 42}, "k"); err == nil {
		t.Fatal("RequiredString wrong-type returned no error")
	}
}

func TestOptionalIntCoerces(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want int
	}{
		{"float64", float64(7), 7},
		{"int", int(7), 7},
		{"int64", int64(7), 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ptr, err := OptionalInt(map[string]any{"k": tc.val}, "k")
			if err != nil {
				t.Fatalf("OptionalInt(%v) err: %v", tc.val, err)
			}
			if ptr == nil || *ptr != tc.want {
				t.Fatalf("OptionalInt(%v) = %v, want %d", tc.val, ptr, tc.want)
			}
		})
	}

	ptr, err := OptionalInt(map[string]any{}, "k")
	if err != nil || ptr != nil {
		t.Fatalf("OptionalInt absent: ptr=%v err=%v", ptr, err)
	}
}

func TestOptionalStringSliceAcceptsBothShapes(t *testing.T) {
	ptr, err := OptionalStringSlice(map[string]any{"k": []string{"a", "b"}}, "k")
	if err != nil || ptr == nil || len(*ptr) != 2 || (*ptr)[0] != "a" {
		t.Fatalf("[]string shape: ptr=%v err=%v", ptr, err)
	}

	ptr, err = OptionalStringSlice(map[string]any{"k": []any{"a", "b"}}, "k")
	if err != nil || ptr == nil || len(*ptr) != 2 || (*ptr)[1] != "b" {
		t.Fatalf("[]any shape: ptr=%v err=%v", ptr, err)
	}

	if _, err := OptionalStringSlice(map[string]any{"k": []any{1, 2}}, "k"); err == nil {
		t.Fatal("OptionalStringSlice with non-strings returned no error")
	}
}

func TestSplitTextLinesPreservesNewlines(t *testing.T) {
	in := "a\nb\nc"
	lines := SplitTextLines(in)
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// Reassembly should round-trip exactly.
	rebuilt := ""
	for _, l := range lines {
		rebuilt += l
	}
	if rebuilt != in {
		t.Fatalf("round-trip mismatch: got %q want %q", rebuilt, in)
	}

	// Trailing newline shouldn't add a phantom empty line.
	lines = SplitTextLines("a\n")
	if len(lines) != 1 || lines[0] != "a\n" {
		t.Fatalf("a\\n -> %v, want [\"a\\n\"]", lines)
	}

	if got := SplitTextLines(""); len(got) != 0 {
		t.Fatalf("empty -> %v, want []", got)
	}
}

func TestApplyTextPatchReplace(t *testing.T) {
	original := "hello world\nfoo bar\n"
	out, meta, err := ApplyTextPatch(original, FilePatchOp{
		Op:  "replace",
		Old: "world",
		New: "everyone",
	})
	if err != nil {
		t.Fatalf("ApplyTextPatch returned err: %v", err)
	}
	want := "hello everyone\nfoo bar\n"
	if out != want {
		t.Fatalf("ApplyTextPatch = %q, want %q", out, want)
	}
	if meta["op"] != "replace" {
		t.Fatalf("meta[op] = %v, want replace", meta["op"])
	}
}

func TestApplyTextPatchAmbiguousReplaceErrors(t *testing.T) {
	_, _, err := ApplyTextPatch("foo\nfoo\n", FilePatchOp{Op: "replace", Old: "foo", New: "bar"})
	if err == nil {
		t.Fatal("expected error for ambiguous replace, got nil")
	}
}

func TestApplyTextPatchInsertAtLine(t *testing.T) {
	original := "line1\nline2\nline3\n"
	startLine := 1
	out, _, err := ApplyTextPatch(original, FilePatchOp{
		Op:        "insert",
		StartLine: &startLine,
		New:       "inserted\n",
	})
	if err != nil {
		t.Fatalf("ApplyTextPatch returned err: %v", err)
	}
	want := "line1\ninserted\nline2\nline3\n"
	if out != want {
		t.Fatalf("ApplyTextPatch = %q, want %q", out, want)
	}
}

func TestApplyTextPatchDeleteLines(t *testing.T) {
	original := "line1\nline2\nline3\nline4\n"
	start, end := 2, 3
	out, _, err := ApplyTextPatch(original, FilePatchOp{
		Op:        "delete",
		StartLine: &start,
		EndLine:   &end,
	})
	if err != nil {
		t.Fatalf("ApplyTextPatch returned err: %v", err)
	}
	want := "line1\nline4\n"
	if out != want {
		t.Fatalf("ApplyTextPatch = %q, want %q", out, want)
	}
}

func TestTextSHA256IsStable(t *testing.T) {
	got := TextSHA256("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("TextSHA256(%q) = %s, want %s", "hello", got, want)
	}
}
