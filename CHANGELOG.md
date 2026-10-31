# Changelog

## [0.4.2](https://github.com/0xfnzero/safe-devops/releases/tag/v0.4.2) — 2026-10-31

Replace the default Electron artwork with the safe-devops shield and terminal logo across the desktop client and Web console.

- Native macOS ICNS, Windows ICO and Linux PNG icons, including multiple sizes and Retina images.
- New launcher, console, browser favicon and Apple touch icons; the macOS development client also sets its Dock icon.
- Editable SVG originals and `make icons` to regenerate all platform assets. Source bundles include the branding files.
- Consistent centered English/Chinese README headers following the sol-trade-sdk layout, without the extra version/navigation line.
- Updated package versions and deployment examples to 0.4.2.

Desktop installers remain unsigned and macOS builds are not notarized. This release does not change command review or device authentication policy.

## [0.4.1](https://github.com/0xfnzero/safe-devops/releases/tag/v0.4.1) — 2026-10-31

First public release of safe-devops, an open-source, self-hosted DevSecOps access control platform for development and operations teams.

- Go Broker, outbound Server Connector, independent AI Reviewer, Next.js console and Electron client.
- General shell commands with exact single-use approval, scoped diagnostics, installed tasks and approved change plans; rule-based, AI and independent human review.
- Device-bound Ed25519 request signatures, replay prevention and optional IP restrictions.
- Recent-operation risk assessments, account containment and independently signed recovery.
- Multi-tenant teams, project-scoped membership and multiple servers per project.
- SQLite by default, optional PostgreSQL, TLS/mTLS deployment, backup and restore.
- Continuous terminal input/output, command completion, history search and scoped sessions.
- Chinese/English and dark/light interfaces; shared accessible dropdown menus and single-border input focus.
- Discoverable Make commands, cross-platform CLI and desktop packaging, Compose and systemd tooling.
- Bilingual README, searchable repository metadata, release checksums and public contribution/security guidance.

This is an early public release. It has not undergone an independent security audit. General shell execution requires an installed executor and exact per-command review; full-screen PTY sessions are not supported. Desktop distributions are unsigned unless explicitly built in a signing environment. See [validation](docs/validation.md), [security boundaries](docs/architecture.md) and each release's platform notes.
