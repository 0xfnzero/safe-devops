# Security policy

safe-devops v0.4 is a security-sensitive self-hosted project with deployment tooling. It has automated negative tests, but has not received an independent security audit. Do not deploy it as your only production access control.

Report vulnerabilities privately using [GitHub Security Advisories](https://github.com/0xfnzero/safe-devops/security/advisories/new) (**Security → Report a vulnerability**). If the form is unavailable, ask a maintainer for a private channel without posting vulnerability details, keys or production output in public issues.

Include the affected commit/version, a minimal local reproduction, expected and observed behavior, and which trust assumptions were violated. Use demo identities and synthetic data only. Please do not test against someone else's infrastructure.

Key security assumptions and known limits are in `docs/architecture.md`. A compromised Connector host, compromised approval quorum, writable executable/runbook, unprotected alternative SSH/cloud access, or approval from the same compromised device is outside the isolation guarantee.

Automatic low-risk tasks intentionally need no reviewer signature. AI reviewers can make mistakes; their keys can authorize only locally permitted AI tasks. High-risk tasks require humans. A compromised requester can use preauthorized tasks and remaining uses of valid grants. See `docs/ai-review.md`.

Human APIs in device-enforced workspaces require a fresh identity-bound device signature on every request. This is software key possession proof, not hardware attestation. Stolen decrypted device keys or a controlled enrolled machine remain a risk. See `docs/device-access.md`.

The AI model is an untrusted decision source with no execution tools or signing keys. Independent Go policy, Broker authorization and Connector checks limit its authority. A malicious model can still misapprove within installed AI scopes or trigger temporary account locks; confidence is not a security proof. See `docs/ai-review.md` for enforced guardrails and containment limits.
