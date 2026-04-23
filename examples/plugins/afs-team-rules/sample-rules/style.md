# Style

- Prefer explicit over clever. Name things for what they do, not what they are.
- No comments that describe WHAT code does. Only comments that explain WHY a
  non-obvious decision was made.
- Max line length: 100 chars.
- Imports: standard library first, then third-party, then local. One group per
  block, blank line between.
- Don't write helpers for a single caller. Inline it.
- Don't add configuration knobs for hypothetical future needs. Add them when a
  second caller appears.
