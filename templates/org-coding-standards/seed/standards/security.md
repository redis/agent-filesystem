# Security standards

Replace with your team's rules. Defaults below.

## Secrets

- Never commit secrets. Use the org secret manager.
- Tokens in logs must be redacted.

## Input validation

- Validate all inputs at trust boundaries (HTTP handlers, CLI args,
  external API responses).
- Parameterize all SQL; never string-concatenate queries.

## Authorization

- Default deny. Explicitly allow per endpoint and per action.
- Never trust client-supplied user identity.
