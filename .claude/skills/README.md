# Agent skills

Go skills for coding agents (Claude Code loads them from here), vendored from
[samber/cc-skills-golang](https://github.com/samber/cc-skills-golang) under its
MIT license ([LICENSE](LICENSE)). Only the skills that fit templatic are kept:
no database, RPC, DI-framework or third-party-library skills. Where a skill
and [AGENTS.md](../../AGENTS.md) disagree, AGENTS.md wins.

To update, copy the newer skill directories over these (without `evals/`).
