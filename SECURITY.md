# Security policy

Acksy is in early development. There are no releases with long-term support
yet: fixes land on `main` and in the `ghcr.io/oowliq/acksy:main` image.

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:
**Security → Report a vulnerability** in this repository
(<https://github.com/oowliq/acksy/security/advisories/new>).
Do not open a public issue for security problems.

Include what you found, how to reproduce it and what an attacker gains. You
will get an answer within a week.

## What is checked automatically

- Go and web tests, including Postgres and Redis integration tests.
- `govulncheck` for Go, `pnpm audit` for web dependencies.
- CodeQL for Go, TypeScript and the GitHub Actions workflows.
- gitleaks for secrets in the git history.
- Trivy for the Docker image before it is published.
- Dependabot for Go modules, npm packages, actions and base images.

The security model (who may do what, how tokens, webhooks and HTTP actions
are restricted) is described in [AGENTS.md](AGENTS.md#безопасность) and the
ADRs in [docs/adr](docs/adr).
