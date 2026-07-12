---
name: taskflow-parallel-tickets
description: Orchestrating parallel multi-ticket implementation with taskflow self-healing flows and git worktrees. Load when running taskflow to implement several tickets/slices at once, when review phases stall under rate limits, or when merging parallel worktree branches back to main.
---

# Taskflow parallel-ticket orchestration

Proven mechanics from building herdr-web-tui (8 tickets, self-healing taskflow + worktrees). Follow these; they are paid-for lessons, not theory.

## Rate limits: reviews are the bottleneck, serialize them
- **N parallel review phases saturate the account rate limit.** A single reviewer subagent then crawls for 30–60+ min on one 429-backoff call while making ~0 progress (0.4% CPU, frozen phase). 4 parallel reviews once cost >1h/ticket; the same reviews at `concurrency: 1` finished in 7–8 min once quota eased.
- **Rule: any flow with multiple LLM review/gate phases → set `"concurrency": 1`.** Parallelize *implement* (worktrees isolate files), serialize *review*.
- If reviews are already thrashing: kill the runner + its child `pi` subagents, checkpoint each worktree to its branch (git, no LLM), then run a separate `concurrency: 1` review-only flow when quota is healthy.

## build-test "done" ≠ green
- A `type:script` build-test phase exits 0 by design (it echoes `RESULT=GREEN`/`RESULT=RED`). Taskflow marks it **done** either way. **Grep the output for `RESULT=GREEN`** — never trust phase status alone. A "done" build-test with `RESULT=RED` means the code is broken and the review is looping to fix it.
- For race-prone Go tests add `go test -race ./...` to build-test; plain `go test` passing is not proof (a racy test passes ~50% of runs).

## Cross-worktree merges: git won't catch semantic conflicts
- Each branch building green in isolation does **not** mean green after merge. Real breakages seen: a widened function signature (`New(fs,log)` → `New(fs,log,herdr,dir)`) leaving old callers/tests behind; a package-level symbol (`defaultSession`) redeclared in two branches; interface members added on both sides.
- **Orchestrator MUST integrate + run the combined build after merging.** Merge branches into main one at a time; after each, run `frontend build && go vet && go build && go test && vitest`.
- Frontend branches that all touch `frontend/src` (App.svelte, terminal.ts) conflict with each other, not with a backend branch. Most are **keep-both** (each side added a different import / interface member / component) — no logic choice. For interleaved keep-both in a hot-path file, delegate resolution to a worker (resolve + build + verify) rather than hand-merging; for a one-line keep-both, resolve inline.

## Agent gotchas
- **`doc-writer` agentType fails in taskflow**: `Model "{{fast}}" not found` (its model alias isn't resolved by the taskflow runner). Use **`support`** for docs/synthesis instead.
- Self-healing Archetype 2 works well: `scout(scout-model) → implement(worker) → build-test(script) → review(gate, onBlock:retry ×2-3 re-runs implement+build-test) → commit(final)`. No hand-written loop. Gate is the sole path to commit → a persistent block halts without committing broken code (intended; the sole `gate-exhaustion` verify warning is expected).

## Worktree lifecycle
- `git worktree add -b ticket-N ../repo-tN HEAD` per parallel ticket, branched off the last merged commit.
- After merge: `git worktree remove ../repo-tN --force` + `git branch -d ticket-N`.
- Add `.pi/taskflows/runs` to `.gitignore` (run-state churn).
- Watch a detached run by polling its run-state JSON at `.pi/taskflows/runs/<name>/<runId>.json` for `status in {completed,failed,blocked}`; a phase `frozen_min` (now − updatedAt) that keeps climbing with a live low-CPU child `pi` = rate-limit crawl, not progress.
