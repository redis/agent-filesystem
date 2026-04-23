# Architecture

- One module, one responsibility. If a module's name has "and" in it, split.
- Dependencies flow downward: UI → service → repo → storage. No upward calls.
- Cross-boundary changes require an ADR in `/done/adr-<slug>.md` of the
  `team-prd` workspace.
- Avoid premature abstraction. Three concrete uses before extracting.
- Prefer composition over inheritance. Inheritance for "is-a"; composition for
  "has-a".
