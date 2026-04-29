# TypeScript

Replace this file with your team's TypeScript standards.

## Types

- Prefer `type` aliases for unions and discriminated unions, `interface`
  for public object contracts that might be extended.
- Never use `any`. Use `unknown` at boundaries and narrow.

## React components

- Components are functions. No classes.
- Keep prop types colocated with the component file.
- State flows down, events flow up. Avoid shared mutable singletons.

## Async

- Prefer `async/await` over raw promise chains.
- Always handle rejection — either `try/catch` or route through a
  boundary that does.
