---
name: release-deploy
description: Route Herdr Web TUI release and deployment requests without conflating them. Use when the user says release, deploy, ship, cut a tag, publish, test a deployment, or asks Config-Management to roll out a revision. Release publishes a version to GitHub; deployment delegates a selected revision to Config-Management and may remain private/untagged.
---

# Release and deployment routing

This repo ships as a **Nix flake** (`flake.nix` → `nix/package.nix`). Keep two operations distinct:

- **Release** publishes a versioned revision to GitHub for downstream consumers.
- **Deployment** applies a selected revision through the `Config-Management` agent for runtime validation or use. It may pin an untagged commit and does not imply a Release.

## Request routing

- `deploy` → discover the exact `Config-Management` mesh address, then delegate pin/build/switch/service/HTTP/log verification. Do not tag, bump version, or publish unless separately requested.
- `release` → run the GitHub Release workflow below. Do not deploy unless separately requested.
- `release and deploy` → perform both explicit operations; state the order before acting.
- Deployment serves as pragmatic runtime testing because this project lacks separate test infrastructure and manpower. Report it as deployment validation, not a full staging test program.
- Technical health does not equal user acceptance. After a healthy Deployment, report evidence and wait for the Owner to deem it acceptable. Once accepted, offer to create a Release; never create one automatically.

## Deployment workflow

1. Resolve target revision from the request; default to current committed HEAD when no tag is required.
2. Load the agent-network skill, list peers, and echo the `Config-Management` address verbatim.
3. Delegate a lock-only pin where possible, repository-required build, switch, and runtime checks.
4. Require active executable identity, service state, HTTP health, and a bounded log observation covering subscription stability/errors.
5. On failure, require rollback and withhold config commit/push. On success, allow the Config-Management agent to commit/push its scoped change.
6. Report results. If the Owner later accepts the Deployment, offer the Release workflow.

## Release scope

Release means: land the fix on `main`, refresh Nix hashes if dependencies changed, bump version, create an annotated tag, and push `main` plus tag. Host rebuild, config pinning, and service restart remain Deployment work.

## When hashes must change
`nix/package.nix` has two content hashes that pin dependency trees:
- `vendorHash` — Go module deps. **Recompute only if `go.mod`/`go.sum` changed.**
- `npmDepsHash` — npm deps. **Recompute only if `frontend/package-lock.json`
  (or `package.json` deps) changed.**

Frontend/backend *source* edits (`.svelte`, `.ts`, `.go`, `index.html`) do NOT
change either hash. Check first:

```sh
git diff --name-only <last-release-tag-or-rev>..HEAD | grep -E 'go\.mod|go\.sum|package-lock\.json|package\.json'
```

No matches → skip the hash step entirely.

## Updating a hash (only when the check above matches)
Nix's fixed-output-derivation trick: set the hash to empty, let the build fail,
copy the "got:" hash back in.

```sh
# vendorHash (Go):
#   set   vendorHash = "";   in nix/package.nix, then:
nix build .#herdr-web-tui 2>&1 | grep -A2 'got:'
#   paste the got: sha256-... into vendorHash.

# npmDepsHash (frontend): same dance with npmDepsHash = "";
# (prefetch alternative: nix run nixpkgs#prefetch-npm-deps -- frontend/package-lock.json)
```
Re-run `nix build .#herdr-web-tui` until it's green before tagging.

## Version bump
Bump `version = "x.y.z";` in `nix/package.nix` (two occurrences: the `frontend`
buildNpmPackage and the outer buildGoModule — keep them in sync). Semver: patch
for a fix, minor for a feature. Commit with the hash change (if any).

## Tag + push
```sh
git tag -a vX.Y.Z -m "vX.Y.Z: <one-line summary>"
git push origin main --follow-tags     # pushes main + the new tag together
```
`--follow-tags` pushes annotated tags reachable from the pushed commits in one
shot. (Or `git push origin main && git push origin vX.Y.Z`.)

## Release checklist
1. Fix is committed + green on `main` (`vite build`, `go build/test` pass).
2. Dep-file diff check → update `vendorHash`/`npmDepsHash` only if needed;
   `nix build .#herdr-web-tui --no-link` green.
3. Bump `version` in `nix/package.nix` (both spots).
4. `git tag -a vX.Y.Z` + `git push origin main --follow-tags`.
5. Report published tag. Deployment remains separate unless explicitly requested.
