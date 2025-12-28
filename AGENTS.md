# Repository Guidelines

## Project Structure & Module Organization

- `main.go` is the CLI entry point.
- `internal/` holds core packages (config, manifest parsing, source fetching, UI/logging, and state).
- `internal/ghpm/` implements manager logic split across install, planning, and archive helpers.
- `doc/` provides user-facing references (see `doc/manifest-reference.md`).
- `doc/design.md` captures higher-level architecture notes.

## Build, Test, and Development Commands

- `go build ./cmd/ghpm` builds the `ghpm` binary.
- `go test ./...` runs all Go tests (none exist yet; add as features grow).
- `go vet ./...` performs static checks before submitting changes.

## Coding Style & Naming Conventions

- Go standard formatting is required: run `gofmt` on all `.go` files.
- Use tabs for indentation (Go default), and keep line lengths readable.
- Package names should be short and lower-case (e.g., `config`, `manifest`).
- File names use lower-case with underscores only when needed (e.g., `manifest.go`).

## Testing Guidelines

- Tests should live alongside code as `*_test.go`.
- Prefer table-driven tests for manifest parsing and source behaviors.
- Name tests after the behavior under test (e.g., `TestLoadConfigDefaults`).
- Run `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines

- No commit history exists yet; use clear, imperative summaries (e.g., "Add manifest URL install").
- Keep commits focused; avoid mixing refactors with behavior changes.
- PRs should include: a short problem statement, approach, and any CLI/output changes.
- Link related issues if applicable and note any manual test steps.

## Configuration & Security Notes

- Runtime config defaults live in `internal/config/` and the documented path is
  `/etc/ghpm/config.yaml`.
- Be cautious when touching install targets like `/usr/local` or `/etc`; use
  test fixtures or temp dirs when possible.
