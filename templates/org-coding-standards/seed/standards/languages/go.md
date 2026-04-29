# Go

Replace this file with your team's Go standards. The template seeds a
minimal starting point.

## Structure

- Follow `gofmt` and `goimports` output. Do not hand-wrap.
- One concept per package. Resist the urge to create a `util` package.

## Error handling

- Wrap errors with `fmt.Errorf("context: %w", err)` when crossing a
  layer boundary.
- Never ignore errors without an explicit comment.

## Tests

- Table-driven tests where there is more than one input shape.
- Prefer `t.TempDir()` over manual temp-file cleanup.
