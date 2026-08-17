# Issue tracker: GitHub

Issues and specs for this repo live in GitHub Issues. Use `gh` from this clone so repository identity comes from `origin`.

Existing issues and `.scratch/` records are historical input only: do not migrate, relabel, close, or rewrite them unless explicitly requested.

## Operations

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`

Use heredocs for multiline bodies. Fetch labels and comments when a skill needs ticket context.

## Pull requests as a triage surface

**PRs as a request surface: no.**

## Skill conventions

- “Publish to the issue tracker” means create a GitHub issue.
- “Fetch the relevant ticket” means run `gh issue view <number> --comments`.
- Skills must not infer work from existing issues unless the user supplies or selects one.

## Wayfinding

Use one issue labelled `wayfinder:map` as the map and GitHub sub-issues as child tickets. Child labels use `wayfinder:<type>` where type is `research`, `prototype`, `grilling`, or `task`.

Prefer GitHub native issue dependencies for blocking. Claim work with `gh issue edit <number> --add-assignee @me`; resolve by commenting the result, closing the child, then updating the map’s decisions.
