# Architecture principles

Replace with your team's principles. Defaults below.

## Simplicity first

- Prefer the simplest thing that works. Abstractions earn their way in
  by being demanded a second time, not predicted.
- Delete unused code. Do not keep it "just in case".

## Observability

- Log at boundaries with structured fields. Log IDs, not payloads.
- Metrics count things that matter to a human, not every function call.

## Failure

- Make failures explicit at boundaries. Don't swallow errors inside
  helpers that can't recover.
- Retries and timeouts are features. Design them in, don't add them after.
