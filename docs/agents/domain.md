# Domain docs

This repo uses a single domain context.

## Before exploring

- Read root `CONTEXT.md` for canonical vocabulary.
- Read relevant ADRs under `docs/adr/` when that directory exists.
- If either path is absent, proceed silently.

## Vocabulary

Use terms exactly as defined in `CONTEXT.md` in issue titles, specs, hypotheses, refactors, and tests. Avoid synonyms listed there.

If a needed concept is missing, reconsider whether it is project-specific. Record genuine domain gaps through the domain-modeling workflow instead of inventing competing language.

## Decisions

Surface conflicts with an existing ADR explicitly. Create ADRs only for decisions that are hard to reverse, surprising without context, and selected through a real trade-off.
