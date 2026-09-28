# Commit Conventions

> All commits in this repository **must be in English**.
> This document is enforced by agents via `.cursor/rules/commit-conventions.mdc`.

## Format — Conventional Commits

```
<type>(<scope>): <subject>

[optional body]

[optional footer(s)]
```

### Types

| Type | When to use |
|------|-------------|
| `feat` | New user-facing feature or use case |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding or updating tests (no production code) |
| `refactor` | Code change that neither fixes nor adds feature |
| `perf` | Performance improvement |
| `chore` | Build, CI, dependencies, tooling |
| `style` | Formatting, no logic change |
| `build` | Gradle, dependencies, version bumps |
| `ci` | CI/CD configuration |

### Scopes (project modules)

| Scope | Module / area |
|-------|---------------|
| `domain` | `:domain` — entities, rules, events |
| `application` | `:application` — use cases |
| `data` | `:data` — Room, network, parsers |
| `ui` | `:app` — Compose, ViewModels, theme |
| `i18n` | String resources, locales |
| `docs` | Markdown documentation |
| `cursor` | `.cursor/rules/` |
| `sync` | WorkManager, ANP download pipeline |
| `parser` | XLSX import |

Use lowercase. Omit scope if change spans multiple modules: `feat: add municipality search`.

### Subject rules

- Imperative mood: **add**, **fix**, **update** (not "added", "fixes")
- Max **72 characters**
- No period at the end
- No emoji

### Body (optional but recommended for non-trivial changes)

- Explain **why**, not what (the diff shows what)
- Wrap at 72 characters
- Reference use case: `Implements UC-004 municipality search.`
- Reference business rule: `Enforces BR-007 minimum search length.`

### Footer

- `Refs: UC-005` — use case reference
- `Refs: BR-003` — business rule reference
- `BREAKING CHANGE:` — incompatible API/DB migration
- `Closes #123` — issue tracker (when applicable)

## Examples

```
feat(domain): add SurveyWeek value object with BR-001 validation

Refs: BR-001
```

```
fix(sync): preserve cache when ANP download fails

Sync failures no longer clear previously imported weeks.
Enforces BR-011.

Refs: BR-011, UC-001
```

```
docs(use-cases): document UC-003 location selection flow

Refs: UC-003
```

```
test(domain): add FuelProduct normalization tests

Refs: BR-002
```

## Branch naming

```
<type>/<short-description>
```

Examples:
- `feat/municipality-search`
- `fix/sync-cache-preservation`
- `docs/user-business-logic`

## Pull request title

Same format as commit subject. PR description must include:

```markdown
## Summary
- Bullet points of changes

## Use cases
- UC-00X (if applicable)

## Test plan
- [ ] Domain unit tests pass
- [ ] Manual verification steps
```

## Pre-commit checklist

Before committing:

- [ ] Commit message is in **English**
- [ ] Type and scope are correct
- [ ] Domain changes have unit tests (TDD)
- [ ] No secrets, keystores, or `local.properties`
- [ ] No hardcoded user-visible strings (i18n keys used)
- [ ] `docs/glossary.md` updated if domain terms changed
- [ ] Use case doc exists if implementing new user flow

## What NOT to commit

- `local.properties`, `*.keystore`, `.env`
- Build outputs (`build/`, `*.apk`)
- IDE-specific files (`.idea/` except shared configs)
- Large downloaded ANP files in `data/downloads/`
- Agent temp files (`agent-tools/`)

## Agent instructions

When creating commits for the user:
1. Use Conventional Commits format above.
2. Never commit unless explicitly requested.
3. One logical change per commit when possible.
4. Reference UC/BR IDs in body when relevant.
