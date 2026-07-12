---
name: release-deploy
description: Cut a release of herdr-web-tui so downstream Nix flake consumers pick up the change. Load when the user asks to release, deploy, ship, cut a tag, or bump the version after merging fixes to main. Covers updating vendorHash/npmDepsHash in nix/package.nix and tagging + pushing.
---

# Release / deploy herdr-web-tui

This repo ships as a **Nix flake** (`flake.nix` → `nix/package.nix`). Consumers
pin `github:tigorlazuardi/herdr-web-tui` and rebuild to get changes. A "release"
here means: land the fix on `main`, refresh Nix hashes if deps changed, tag, and
push tag — nothing more. **The actual host deploy (home-manager / nixos-rebuild)
is out of scope — the operator does that on their box.**

## Scope boundary
- IN scope: commit fix to `main`, update hashes in `nix/package.nix`, bump
  version, create + push git tag.
- OUT of scope: `nixos-rebuild`, `home-manager switch`, `nix flake update` in any
  consumer repo, restarting the prod service. Do NOT touch those.
  (`systemctl --user restart` does nothing useful here — prod runs the Nix-built
  binary from `/nix/store`, not a local build.)

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

## Full checklist
1. Fix is committed + green on `main` (`vite build`, `go build/test` pass).
2. Dep-file diff check → update `vendorHash`/`npmDepsHash` only if needed;
   `nix build .#herdr-web-tui` green.
3. Bump `version` in `nix/package.nix` (both spots).
4. `git tag -a vX.Y.Z` + `git push origin main --follow-tags`.
5. Tell the operator the tag is up; they rebuild their host themselves.
