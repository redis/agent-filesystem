# Security

- Never log secrets, session tokens, or PII. Redact before logging.
- All user input is untrusted — validate at the boundary, never trust internal
  callers' "pre-validated" claims.
- Use parameterized queries for every database call. No string concatenation
  into SQL, ever.
- Store secrets in the platform's secret manager (not env vars baked into
  images, not config files in the repo).
- Default to deny. Explicit allow-lists beat implicit patterns.
- Auth code requires a second reviewer before merge.
