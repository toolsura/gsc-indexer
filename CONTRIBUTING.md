# Contributing to gsc-indexer

Thank you for considering contributing! This project welcomes contributions of all kinds.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/gsc-indexer`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run the checks: `make check`
6. Commit and push: `git push origin my-feature`
7. Open a Pull Request

## Development Setup

### Prerequisites
- Go 1.24+
- Make
- golangci-lint, staticcheck, gosec (installed via `make tools`)

### Quick Start
```bash
make deps       # Download dependencies
make tools      # Install dev tools
make check      # Run all checks (fmt, vet, staticcheck, gosec, lint, test)
make build      # Build binary
```

## Code Standards

- **Formatting**: `gofmt` / `goimports` (enforced by CI)
- **Linting**: `golangci-lint` with config in `.golangci.yml`
- **Static analysis**: `staticcheck`, `go vet`
- **Security**: `gosec`
- **Tests**: Must pass with `-race`, coverage >= 80%
- **Commits**: Conventional Commits format (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`)

## Pull Request Guidelines

1. **One logical change per PR** — keep it focused
2. **Update tests** — new features need tests, bug fixes need regression tests
3. **Update docs** — README, CHANGELOG, or code comments if behavior changes
4. **Pass CI** — all checks must pass before merge
5. **Small PRs preferred** — easier to review, faster to merge

## Reporting Issues

- Use the issue templates for bugs and feature requests
- Search existing issues first to avoid duplicates
- Include Go version, OS, and steps to reproduce for bugs

## Security

If you discover a security vulnerability, please report it privately via email to security@toolsura.com rather than opening a public issue.

## Code of Conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). By participating, you agree to uphold this code.