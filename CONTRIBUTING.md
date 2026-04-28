# Contributing

Thanks for considering contributing to MC-Starter.

## Quick Start

```bash
git clone https://github.com/tanjunqian666-glitch/mc-starter.git
cd mc-starter
make build
```

## Branch Strategy

- `main` — stable, production-ready
- `feat/*` — new features
- `fix/*` — bug fixes
- `chore/*` — tooling, deps, docs

Submit PRs against `main`. Keep PRs small and focused.

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` — new feature
- `fix:` — bug fix
- `refactor:` — code change without fix or feature
- `docs:` — documentation only
- `chore:` — tooling, CI, deps
- `perf:` — performance improvements
- `style:` — formatting, lint

## Code Style

- `gofumpt` format
- Standard Go error handling (no panics)
- Only use third-party deps when stdlib won't do
- Windows only; avoid platform checks where possible

## Testing

- Unit tests alongside code (`_test.go`)
- `make test` before pushing
- Integration tests in `test/` for Windows-only workflows

## Questions

Open a [Discussion](https://github.com/tanjunqian666-glitch/mc-starter/discussions).
