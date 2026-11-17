# Contributing to Abastevo

Thanks for helping. The working rules for people and coding agents are the
same and live in [AGENTS.md](AGENTS.md): repository map, architecture rules,
commands and the delivery workflow. Read it first.

## Workflow

1. Open or pick an issue, or a roadmap item from [ROADMAP.md](ROADMAP.md).
2. Branch from `dev` (or fork). Keep the change bounded to one concern.
3. Write the rule or test first, then the code. Domain logic is test-first.
4. Run the targeted tests plus `make quick-verify` before pushing.
5. Open a pull request against `main` (maintainers integrate through `dev`).
   Every required check must be green before merge.

## Pull requests

```markdown
## Summary
What changes and why.

## Test plan
Commands run and their results; what was not covered.
```

- English only in code, comments, docs, commits and PR text.
- Conventional Commits: `feat(scope): imperative subject`, at most 72
  characters, one logical change per commit.
- User-facing text goes through string resources (Android) or the content
  files (landing); never hardcode UI text.
- Update the doc that owns a rule or runbook in the same PR.

## Reporting issues

Include the app version or commit, device/OS (Android) or endpoint (backend),
steps to reproduce, expected and actual behavior. Never paste secrets, tokens,
personal data or precise locations. Report security problems privately as
described in [docs/security/security-privacy.md](docs/security/security-privacy.md).

## License

By contributing you agree that your contribution is licensed under the
[Abastevo Visible Attribution License 1.0](LICENSE). Keep the original MIT
notices in [LICENSES/](LICENSES/) and the third-party notices intact.
