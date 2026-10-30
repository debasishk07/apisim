# Contributing to apisim

Thanks for contributing! apisim is a Go project. Keep packages small, avoid
global state, pass dependencies explicitly, and use an injectable clock/RNG so
everything is deterministic and testable.

## Getting started

```
make build        # build the binary
make test         # run tests with -race
make lint         # run golangci-lint
make ci           # vet + lint + test (mirrors CI)
```

## Conventions

- Commit messages: conventional commits (`feat:`, `fix:`, `chore:`, `docs:`).
- One feature/fix per PR; include tests.
- New dependencies must be justified in docs/DECISIONS.md.
- Run `make ci` before pushing.
- Never log secrets; redact sensitive headers by default.

## Reporting issues

Describe the scenario file, the failure you expected, and what actually
happened. Include `apisim version` output.
