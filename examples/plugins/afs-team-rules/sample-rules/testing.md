# Testing

- Every bug fix ships with a regression test that fails before the fix and
  passes after.
- Integration tests must hit a real database (containerized is fine). No mocks
  for SQL, Redis, or HTTP clients crossing service boundaries.
- Unit tests may mock narrow pure-function collaborators.
- Test names describe the behavior under test, not the function: `rejects
  expired tokens`, not `test_validate_token_1`.
- No `skip` or `xfail` in main. If a test is broken, fix it or delete it.
