# Taskflow engine for /implement — self-healing loop (reusable per ticket)

Verified define: `.scratch/web-tui/tf-ticket-0.json` (a copy runs from /tmp).

## Shape (Archetype 2: implement → verify → rework)

```
scout(claude-scout) → implement(claude-worker, TDD, edit-only, no commit)
  → build-test(script: frontend build + go vet/build/test → RESULT=GREEN|RED)
  → review(gate claude-reviewer, onBlock:retry ×3, output json verdict/reason)
        ↳ BLOCK re-runs implement + build-test with the blocker reasons
  → commit(claude-worker, final)
```

- **Loop** = gate `onBlock:"retry"` + `retry.max:3`. No hand-written loop; the gate
  re-runs its `dependsOn` upstreams and re-evaluates, up to 3 rounds, then halts (blocked).
- **No `eval` short-circuit** on the gate on purpose — we want `/code-review` (Standards + Spec)
  to run every round, not auto-pass on a green build.
- **build-test is a `script`** (zero tokens, ground truth); the `; if $?…` tail makes it exit 0 so a
  red build feeds the gate instead of aborting the run.
- `strictInterpolation:true`, `budget.maxUSD:8` (bump per ticket size).
- Gate-exhaustion warning is intentional: 3 failed rounds → halt, never commit broken code.

## Run

```
Taskflow action=verify defineFile=/tmp/tf-ticket-N.json   # 0 tokens
Taskflow action=run    defineFile=/tmp/tf-ticket-N.json detach=true
```

Watch: `/tf runs`, `/tf peek <runId>`, `/tf peek <runId> <phaseId>`. Resume after a fix: `/tf resume <runId>`.

## Reuse per ticket
Swap the `§N` references, the seam description, and the acceptance-criteria/done list in `scout` +
`implement` + `review`. Parallel frontier (tickets 1 & 5): run two instances, or one define with a
`parallel` phase fanning both, each its own implement→build-test→review→commit chain.
