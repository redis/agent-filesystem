# Review checklist

For every PR, check each item and cite the specific standard when you
flag something.

- [ ] Scope matches the PR description; no drive-by changes.
- [ ] New code follows the relevant `standards/languages/*.md`.
- [ ] No new `any`, `interface{}`, `// @ts-ignore`, or `//nolint`
      without a comment explaining why.
- [ ] Tests cover the new behavior and at least one failure mode.
- [ ] Error messages are specific and actionable.
- [ ] No secrets, tokens, or internal URLs in code or comments.
- [ ] Public API changes are documented.
- [ ] Dependencies added are justified; no drive-by adds.
