# Contributing

Thanks for helping. templatic is in early development, so open an issue
before a large change to agree on the approach.

1. Install [mise](https://mise.jdx.dev), then run `mise install` in the
   repository. It installs the pinned Go, Node and tools.
2. Make the change, with tests for the behavior it adds or fixes.
3. Commit, then run `mise run check`. It runs every check CI runs and
   fails on an uncommitted change.
4. Open a pull request against `main`.

Conventions (what is public, comments, tests, dependencies) are in
[AGENTS.md](AGENTS.md). Security issues: see [SECURITY.md](SECURITY.md).
