# Naming

- Functions: verb phrases. `computeTotal`, `fetchUser`, `isExpired`.
- Booleans: `is*`, `has*`, `should*`, `can*`. No negated names (`notReady`).
- Types/classes: noun phrases, PascalCase.
- Constants: SCREAMING_SNAKE_CASE for true constants; `camelCase` for
  module-level configured values.
- Don't repeat the type in the name: `userList: User[]` not `userListArray`.
- Avoid `*Manager`, `*Helper`, `*Util` — they hide what the thing actually does.
