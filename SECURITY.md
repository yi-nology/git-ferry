# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| latest release | ✅ |
| older releases | ❌ (please upgrade) |

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report privately via one of:

1. **GitHub Security Advisories** — [Report a vulnerability](https://github.com/yi-nology/git-ferry/security/advisories/new) (preferred)
2. Email the maintainers listed in the repository profile

Include as much of the following as you can:

- Description of the issue and its impact
- Steps to reproduce / proof of concept
- Affected version or commit
- Any suggested fix

## What to expect

- We acknowledge reports within **72 hours**
- We aim to provide a fix or mitigation plan within **14 days** for critical issues
- Credit will be given in the advisory unless you prefer to remain anonymous

## Scope notes

GitFerry stores **platform access tokens** (GitHub / GitLab / Gitee / GitLink …) encrypted at rest (`ENCRYPTION_KEY`). Issues involving credential leakage, auth bypass, RCE, or SSRF via clone/proxy URLs are treated as critical.

Out of scope (unless they lead to the above):

- Denial of service via resource exhaustion
- Vulnerabilities in third-party dependencies without a realistic exploit path in GitFerry
- Social engineering of operators
