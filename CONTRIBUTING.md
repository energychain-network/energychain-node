# Contributing

Thank you for taking the time to contribute. This document covers the basics; please open an issue or discussion if anything is unclear.

## Getting started

1. Fork the repository and clone your fork.
2. Create a feature branch off `main`: `git checkout -b feat/<short-name>`.
3. Install dependencies (see [README](./README.md) for the toolchain).
4. Make your change and add tests when appropriate.
5. Run the lint / test suite documented in the README.
6. Open a pull request describing **why** the change is needed; reviewers will help with the **how**.

## Commit messages

We follow the [Conventional Commits](https://www.conventionalcommits.org/) spec:

```
<type>(<scope>): <subject>

<body>

<footer>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, `build`, `ci`.
Keep the subject under 72 characters and the body wrapped at ~100.

## Code style

- **Go** — `gofmt -s`, `go vet`, `golangci-lint run` must all pass.
- **TypeScript / JavaScript** — ESLint + Prettier (config lives in repo).
- **Solidity** — `solhint` + `prettier-plugin-solidity`.

## Testing

Every behavioural change should ship with a test. See the README for how to run the suite locally; CI will run the same commands on every PR.

## Reporting security issues

Please do **not** open public issues for security vulnerabilities. See [SECURITY.md](./SECURITY.md) for the responsible-disclosure process.
