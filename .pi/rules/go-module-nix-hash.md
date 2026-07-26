---
paths:
  - go.mod
  - go.sum
  - nix/package.nix
---

# Go module Nix hash

When `go.mod` or `go.sum` changes, refresh `vendorHash` in `nix/package.nix` from Nix reported hash, then run `nix build .#herdr-web-tui --no-link`. Dependency update is complete only when build passes with committed hash.
