# Contributing

Use Go 1.26+, Node.js 22.18+, npm, Python 3.11+ and GNU Make. Run `make setup`, `make dev`, then `make check`. Keep all generated keys and state under ignored `.safeops/`; never commit live tokens, certificates or database URLs.

Security behavior must come with a negative test: demonstrate the previously possible bypass is denied. Keep human authentication, approval signing, broker signing and Connector authentication separate. Do not add arbitrary shell, user-supplied argv, remote scripts, repository pull or deployment credentials to the PR workflow.

The protocol and trust boundaries are documented in `docs/`. Changes to canonical payloads, signing domains, state persistence or local policy require a documented migration. UI changes must pass TypeScript, lint, production build and a browser walkthrough at desktop and mobile sizes.

Submit focused pull requests. For vulnerabilities, use a private report following `SECURITY.md`; do not publish exploit details in an issue before maintainers have responded.
